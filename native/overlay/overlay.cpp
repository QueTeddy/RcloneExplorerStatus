#include <windows.h>
#include <shobjidl.h>
#include <shlobj.h>
#include <shlwapi.h>
#include <objbase.h>
#include <olectl.h>
#include <atomic>
#include <chrono>
#include <cstdio>
#include <cwctype>
#include <mutex>
#include <new>
#include <string>
#include <vector>

#pragma comment(lib, "ole32.lib")
#pragma comment(lib, "shell32.lib")
#pragma comment(lib, "shlwapi.lib")

namespace {

const CLSID CLSID_RcloneError = {0x2E4F48A1, 0x1E78, 0x4DC9, {0x92, 0x31, 0x54, 0x33, 0x18, 0x01, 0x10, 0x01}};
const CLSID CLSID_RcloneSyncing = {0x2E4F48A2, 0x1E78, 0x4DC9, {0x92, 0x31, 0x54, 0x33, 0x18, 0x01, 0x10, 0x02}};
const CLSID CLSID_RcloneSynced = {0x2E4F48A3, 0x1E78, 0x4DC9, {0x92, 0x31, 0x54, 0x33, 0x18, 0x01, 0x10, 0x03}};

struct OverlaySpec {
    const wchar_t* name;
    const wchar_t* icon;
    const char* state;
    const CLSID* clsid;
    int priority;
};

const OverlaySpec kOverlays[] = {
    {L"   Rclone Status Error", L"error.ico", "error", &CLSID_RcloneError, 0},
    {L"   Rclone Status Syncing", L"syncing.ico", "syncing", &CLSID_RcloneSyncing, 1},
    {L"   Rclone Status Synced", L"synced.ico", "synced", &CLSID_RcloneSynced, 2},
};

const wchar_t* kLegacyOverlayNames[] = {
    L"0 Rclone Status Error",
    L"1 Rclone Status Syncing",
    L"2 Rclone Status Synced",
};

const wchar_t* kOverlayRoot = L"SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Explorer\\ShellIconOverlayIdentifiers";
const wchar_t* kClassRoot = L"SOFTWARE\\Classes\\CLSID\\";
const wchar_t* kApprovedRoot = L"SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Shell Extensions\\Approved";
std::atomic<long> g_activeObjects(0);
HMODULE g_module = nullptr;

enum class OverlayKind { Error, Syncing, Synced };

const OverlaySpec* spec_for(OverlayKind kind) {
    switch (kind) {
    case OverlayKind::Error: return &kOverlays[0];
    case OverlayKind::Syncing: return &kOverlays[1];
    default: return &kOverlays[2];
    }
}

std::wstring guid_text(REFCLSID clsid) {
    wchar_t value[40] = {};
    if (StringFromGUID2(clsid, value, ARRAYSIZE(value)) == 0) return L"";
    return value;
}

HRESULT set_reg_string(HKEY root, const std::wstring& subkey, const wchar_t* name, const std::wstring& value) {
    HKEY key = nullptr;
    LONG result = RegCreateKeyExW(root, subkey.c_str(), 0, nullptr, 0, KEY_WRITE, nullptr, &key, nullptr);
    if (result != ERROR_SUCCESS) return HRESULT_FROM_WIN32(result);
    result = RegSetValueExW(key, name, 0, REG_SZ, reinterpret_cast<const BYTE*>(value.c_str()),
        static_cast<DWORD>((value.size() + 1) * sizeof(wchar_t)));
    RegCloseKey(key);
    return result == ERROR_SUCCESS ? S_OK : HRESULT_FROM_WIN32(result);
}

HRESULT delete_reg_tree(HKEY root, const std::wstring& subkey) {
    LONG result = RegDeleteTreeW(root, subkey.c_str());
    if (result == ERROR_FILE_NOT_FOUND || result == ERROR_PATH_NOT_FOUND) return S_OK;
    return result == ERROR_SUCCESS ? S_OK : HRESULT_FROM_WIN32(result);
}

bool delete_legacy_overlay_keys() {
    for (const wchar_t* name : kLegacyOverlayNames) {
        if (FAILED(delete_reg_tree(HKEY_LOCAL_MACHINE, std::wstring(kOverlayRoot) + L"\\" + name))) return false;
    }
    return true;
}

bool write_registration(const OverlaySpec& spec, const std::wstring& modulePath) {
    const std::wstring clsid = guid_text(*spec.clsid);
    const std::wstring classKey = std::wstring(kClassRoot) + clsid;
    const std::wstring inprocKey = classKey + L"\\InprocServer32";
    if (FAILED(set_reg_string(HKEY_LOCAL_MACHINE, inprocKey, nullptr, modulePath))) return false;
    if (FAILED(set_reg_string(HKEY_LOCAL_MACHINE, inprocKey, L"ThreadingModel", L"Apartment"))) return false;
    if (FAILED(set_reg_string(HKEY_LOCAL_MACHINE, std::wstring(kOverlayRoot) + L"\\" + spec.name, nullptr, clsid))) return false;
    if (FAILED(set_reg_string(HKEY_LOCAL_MACHINE, kApprovedRoot, clsid.c_str(), spec.name))) return false;
    return true;
}

bool delete_registration(const OverlaySpec& spec) {
    const std::wstring clsid = guid_text(*spec.clsid);
    if (FAILED(delete_reg_tree(HKEY_LOCAL_MACHINE, std::wstring(kOverlayRoot) + L"\\" + spec.name))) return false;
    if (FAILED(delete_reg_tree(HKEY_LOCAL_MACHINE, std::wstring(kClassRoot) + clsid))) return false;
    HKEY key = nullptr;
    LONG result = RegOpenKeyExW(HKEY_LOCAL_MACHINE, kApprovedRoot, 0, KEY_SET_VALUE, &key);
    if (result == ERROR_SUCCESS) {
        result = RegDeleteValueW(key, clsid.c_str());
        RegCloseKey(key);
        if (result != ERROR_SUCCESS && result != ERROR_FILE_NOT_FOUND) return false;
    } else if (result != ERROR_FILE_NOT_FOUND && result != ERROR_PATH_NOT_FOUND) {
        return false;
    }
    return true;
}

std::wstring state_file_path() {
    wchar_t localAppData[32768] = {};
    DWORD length = GetEnvironmentVariableW(L"LOCALAPPDATA", localAppData, ARRAYSIZE(localAppData));
    if (length == 0 || length >= ARRAYSIZE(localAppData)) return L"";
    return std::wstring(localAppData) + L"\\RcloneExplorerStatus\\state.json";
}

std::string wide_to_utf8(const std::wstring& value) {
    if (value.empty()) return std::string();
    int length = WideCharToMultiByte(CP_UTF8, 0, value.c_str(), static_cast<int>(value.size()), nullptr, 0, nullptr, nullptr);
    if (length <= 0) return std::string();
    std::string result(static_cast<size_t>(length), '\0');
    WideCharToMultiByte(CP_UTF8, 0, value.c_str(), static_cast<int>(value.size()), &result[0], length, nullptr, nullptr);
    return result;
}

std::string json_quote(const std::string& value) {
    std::string result = "\"";
    for (unsigned char ch : value) {
        switch (ch) {
        case '\\': result += "\\\\"; break;
        case '"': result += "\\\""; break;
        case '\b': result += "\\b"; break;
        case '\f': result += "\\f"; break;
        case '\n': result += "\\n"; break;
        case '\r': result += "\\r"; break;
        case '\t': result += "\\t"; break;
        default:
            if (ch < 0x20) {
                char escaped[7] = {};
                sprintf_s(escaped, "\\u%04x", ch);
                result += escaped;
            } else {
                result += static_cast<char>(ch);
            }
        }
    }
    result += '"';
    return result;
}

std::wstring normalize_path(const wchar_t* rawPath) {
    std::wstring path(rawPath ? rawPath : L"");
    for (wchar_t& ch : path) {
        if (ch == L'/') ch = L'\\';
        ch = towlower(ch);
    }
    if (path.compare(0, 4, L"\\\\?\\") == 0) path.erase(0, 4);
    while (path.size() > 3 && path.back() == L'\\') path.pop_back();
    return path;
}

bool state_from_json(const std::string& json, const std::string& quotedPath, const char* expectedState) {
    size_t key = json.find(quotedPath);
    if (key == std::string::npos) return false;
    size_t colon = json.find(':', key + quotedPath.size());
    if (colon == std::string::npos) return false;
    size_t objectStart = json.find('{', colon + 1);
    if (objectStart == std::string::npos) return false;

    size_t objectEnd = std::string::npos;
    int depth = 0;
    bool inString = false;
    bool escaped = false;
    for (size_t i = objectStart; i < json.size(); ++i) {
        const char ch = json[i];
        if (inString) {
            if (escaped) escaped = false;
            else if (ch == '\\') escaped = true;
            else if (ch == '"') inString = false;
            continue;
        }
        if (ch == '"') inString = true;
        else if (ch == '{') ++depth;
        else if (ch == '}' && --depth == 0) { objectEnd = i; break; }
    }
    if (objectEnd == std::string::npos) return false;

    const std::string stateKey = "\"state\"";
    size_t state = json.find(stateKey, objectStart);
    if (state == std::string::npos || state >= objectEnd) return false;
    size_t stateColon = json.find(':', state + stateKey.size());
    if (stateColon == std::string::npos || stateColon >= objectEnd) return false;
    size_t valueStart = json.find('"', stateColon + 1);
    if (valueStart == std::string::npos || valueStart >= objectEnd) return false;
    size_t valueEnd = json.find('"', valueStart + 1);
    if (valueEnd == std::string::npos || valueEnd >= objectEnd) return false;
    return json.compare(valueStart + 1, valueEnd - valueStart - 1, expectedState) == 0;
}

class StateCache {
public:
    bool has_state(const wchar_t* path, const char* state) {
        const std::wstring normalized = normalize_path(path);
        const std::string quoted = json_quote(wide_to_utf8(normalized));
        const ULONGLONG now = GetTickCount64();
        std::lock_guard<std::mutex> guard(lock_);
        if (now - checkedAt_ >= 250 || !loaded_) reload();
        checkedAt_ = now;
        return !json_.empty() && state_from_json(json_, quoted, state);
    }

private:
    void reload() {
        loaded_ = true;
        json_.clear();
        const std::wstring path = state_file_path();
        if (path.empty()) return;
        FILE* file = nullptr;
        if (_wfopen_s(&file, path.c_str(), L"rb") != 0 || file == nullptr) return;
        char buffer[16384];
        size_t count = 0;
        while ((count = fread(buffer, 1, sizeof(buffer), file)) > 0) json_.append(buffer, count);
        fclose(file);
    }

    std::mutex lock_;
    ULONGLONG checkedAt_ = 0;
    bool loaded_ = false;
    std::string json_;
};

StateCache g_stateCache;

class OverlayHandler final : public IShellIconOverlayIdentifier {
public:
    explicit OverlayHandler(OverlayKind kind) : kind_(kind), refs_(1) { ++g_activeObjects; }
    ~OverlayHandler() { --g_activeObjects; }

    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID riid, void** object) override {
        if (!object) return E_POINTER;
        *object = nullptr;
        if (IsEqualIID(riid, IID_IUnknown) || IsEqualIID(riid, IID_IShellIconOverlayIdentifier)) {
            *object = static_cast<IShellIconOverlayIdentifier*>(this);
            AddRef();
            return S_OK;
        }
        return E_NOINTERFACE;
    }

    ULONG STDMETHODCALLTYPE AddRef() override { return static_cast<ULONG>(InterlockedIncrement(&refs_)); }
    ULONG STDMETHODCALLTYPE Release() override {
        ULONG count = static_cast<ULONG>(InterlockedDecrement(&refs_));
        if (count == 0) delete this;
        return count;
    }

    HRESULT STDMETHODCALLTYPE IsMemberOf(LPCWSTR path, DWORD) override {
        if (!path || !*path) return S_FALSE;
        return g_stateCache.has_state(path, spec_for(kind_)->state) ? S_OK : S_FALSE;
    }

    HRESULT STDMETHODCALLTYPE GetOverlayInfo(LPWSTR iconFile, int cchMax, int* index, DWORD* flags) override {
        if (!iconFile || cchMax <= 0 || !index || !flags) return E_INVALIDARG;
        wchar_t modulePath[32768] = {};
        DWORD length = GetModuleFileNameW(g_module, modulePath, ARRAYSIZE(modulePath));
        if (length == 0 || length >= ARRAYSIZE(modulePath)) return HRESULT_FROM_WIN32(GetLastError());
        PathRemoveFileSpecW(modulePath);
        PathRemoveFileSpecW(modulePath);
        if (!PathAppendW(modulePath, L"assets") || !PathAppendW(modulePath, spec_for(kind_)->icon)) return E_FAIL;
        if (wcscpy_s(iconFile, static_cast<size_t>(cchMax), modulePath) != 0) return HRESULT_FROM_WIN32(ERROR_INSUFFICIENT_BUFFER);
        *index = 0;
        *flags = ISIOI_ICONFILE | ISIOI_ICONINDEX;
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE GetPriority(int* priority) override {
        if (!priority) return E_POINTER;
        *priority = spec_for(kind_)->priority;
        return S_OK;
    }

private:
    OverlayKind kind_;
    volatile LONG refs_;
};

class OverlayFactory final : public IClassFactory {
public:
    explicit OverlayFactory(OverlayKind kind) : kind_(kind), refs_(1) { ++g_activeObjects; }
    ~OverlayFactory() { --g_activeObjects; }

    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID riid, void** object) override {
        if (!object) return E_POINTER;
        *object = nullptr;
        if (IsEqualIID(riid, IID_IUnknown) || IsEqualIID(riid, IID_IClassFactory)) {
            *object = static_cast<IClassFactory*>(this);
            AddRef();
            return S_OK;
        }
        return E_NOINTERFACE;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return static_cast<ULONG>(InterlockedIncrement(&refs_)); }
    ULONG STDMETHODCALLTYPE Release() override {
        ULONG count = static_cast<ULONG>(InterlockedDecrement(&refs_));
        if (count == 0) delete this;
        return count;
    }
    HRESULT STDMETHODCALLTYPE CreateInstance(IUnknown* outer, REFIID riid, void** object) override {
        if (outer) return CLASS_E_NOAGGREGATION;
        OverlayHandler* handler = new (std::nothrow) OverlayHandler(kind_);
        if (!handler) return E_OUTOFMEMORY;
        HRESULT result = handler->QueryInterface(riid, object);
        handler->Release();
        return result;
    }
    HRESULT STDMETHODCALLTYPE LockServer(BOOL) override { return S_OK; }

private:
    OverlayKind kind_;
    volatile LONG refs_;
};

OverlayKind kind_for(REFCLSID clsid, bool* found) {
    *found = true;
    if (IsEqualCLSID(clsid, CLSID_RcloneError)) return OverlayKind::Error;
    if (IsEqualCLSID(clsid, CLSID_RcloneSyncing)) return OverlayKind::Syncing;
    if (IsEqualCLSID(clsid, CLSID_RcloneSynced)) return OverlayKind::Synced;
    *found = false;
    return OverlayKind::Synced;
}

} // namespace

extern "C" BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) {
        g_module = instance;
        DisableThreadLibraryCalls(instance);
    }
    return TRUE;
}

STDAPI DllGetClassObject(REFCLSID clsid, REFIID riid, void** object) {
    if (!object) return E_POINTER;
    *object = nullptr;
    bool found = false;
    OverlayKind kind = kind_for(clsid, &found);
    if (!found) return CLASS_E_CLASSNOTAVAILABLE;
    OverlayFactory* factory = new (std::nothrow) OverlayFactory(kind);
    if (!factory) return E_OUTOFMEMORY;
    HRESULT result = factory->QueryInterface(riid, object);
    factory->Release();
    return result;
}

STDAPI DllCanUnloadNow(void) {
    return g_activeObjects.load() == 0 ? S_OK : S_FALSE;
}

STDAPI DllRegisterServer(void) {
    wchar_t modulePath[32768] = {};
    DWORD length = GetModuleFileNameW(g_module, modulePath, ARRAYSIZE(modulePath));
    if (length == 0 || length >= ARRAYSIZE(modulePath)) return HRESULT_FROM_WIN32(GetLastError());
    if (!delete_legacy_overlay_keys()) return SELFREG_E_CLASS;
    for (const OverlaySpec& spec : kOverlays) {
        if (!write_registration(spec, modulePath)) return SELFREG_E_CLASS;
    }
    SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, nullptr, nullptr);
    return S_OK;
}

STDAPI DllUnregisterServer(void) {
    if (!delete_legacy_overlay_keys()) return SELFREG_E_CLASS;
    for (const OverlaySpec& spec : kOverlays) {
        if (!delete_registration(spec)) return SELFREG_E_CLASS;
    }
    SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, nullptr, nullptr);
    return S_OK;
}
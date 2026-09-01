//go:build darwin && cgo

package secret

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef gum_keychain_query(const char *service, const char *account) {
	CFStringRef serviceValue = CFStringCreateWithCString(kCFAllocatorDefault, service, kCFStringEncodingUTF8);
	CFStringRef accountValue = CFStringCreateWithCString(kCFAllocatorDefault, account, kCFStringEncodingUTF8);
	if (serviceValue == NULL || accountValue == NULL) {
		if (serviceValue != NULL) CFRelease(serviceValue);
		if (accountValue != NULL) CFRelease(accountValue);
		return NULL;
	}
	CFMutableDictionaryRef query = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (query != NULL) {
		CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
		CFDictionarySetValue(query, kSecAttrService, serviceValue);
		CFDictionarySetValue(query, kSecAttrAccount, accountValue);
	}
	CFRelease(serviceValue);
	CFRelease(accountValue);
	return query;
}

static OSStatus gum_keychain_store(const char *service, const char *account, const void *value, CFIndex valueLength) {
	CFMutableDictionaryRef query = gum_keychain_query(service, account);
	if (query == NULL) return errSecAllocate;
	CFDataRef data = CFDataCreate(kCFAllocatorDefault, value, valueLength);
	if (data == NULL) {
		CFRelease(query);
		return errSecAllocate;
	}
	const void *updateKeys[] = { kSecValueData };
	const void *updateValues[] = { data };
	CFDictionaryRef updates = CFDictionaryCreate(kCFAllocatorDefault, updateKeys, updateValues, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus status = updates == NULL ? errSecAllocate : SecItemUpdate(query, updates);
	if (status == errSecItemNotFound) {
		CFDictionarySetValue(query, kSecValueData, data);
		status = SecItemAdd(query, NULL);
	}
	if (updates != NULL) CFRelease(updates);
	CFRelease(data);
	CFRelease(query);
	return status;
}

static OSStatus gum_keychain_resolve(const char *service, const char *account, unsigned char **value, CFIndex *valueLength) {
	CFMutableDictionaryRef query = gum_keychain_query(service, account);
	if (query == NULL) return errSecAllocate;
	CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	CFRelease(query);
	if (status != errSecSuccess) return status;
	if (result == NULL || CFGetTypeID(result) != CFDataGetTypeID()) {
		if (result != NULL) CFRelease(result);
		return errSecInternalComponent;
	}
	CFDataRef data = (CFDataRef)result;
	*valueLength = CFDataGetLength(data);
	size_t allocationLength = *valueLength > 0 ? (size_t)*valueLength : 1;
	*value = malloc(allocationLength);
	if (*value == NULL) {
		CFRelease(result);
		return errSecAllocate;
	}
	if (*valueLength > 0) memcpy(*value, CFDataGetBytePtr(data), (size_t)*valueLength);
	CFRelease(result);
	return errSecSuccess;
}

static OSStatus gum_keychain_delete(const char *service, const char *account) {
	CFMutableDictionaryRef query = gum_keychain_query(service, account);
	if (query == NULL) return errSecAllocate;
	OSStatus status = SecItemDelete(query);
	CFRelease(query);
	return status;
}
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"
)

type securityFrameworkBackend struct{}

func newSecurityFrameworkBackend() KeychainBackend {
	return securityFrameworkBackend{}
}

func (securityFrameworkBackend) Store(ctx context.Context, service, account, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	serviceValue := C.CString(service)
	accountValue := C.CString(account)
	secretValue := C.CBytes([]byte(value))
	defer C.free(unsafe.Pointer(serviceValue))
	defer C.free(unsafe.Pointer(accountValue))
	defer C.free(secretValue)
	status := C.gum_keychain_store(serviceValue, accountValue, secretValue, C.CFIndex(len(value)))
	if status != C.errSecSuccess {
		return fmt.Errorf("keychain store status %d", int32(status))
	}
	return nil
}

func (securityFrameworkBackend) Resolve(ctx context.Context, service, account string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	serviceValue := C.CString(service)
	accountValue := C.CString(account)
	defer C.free(unsafe.Pointer(serviceValue))
	defer C.free(unsafe.Pointer(accountValue))
	var value *C.uchar
	var valueLength C.CFIndex
	status := C.gum_keychain_resolve(serviceValue, accountValue, &value, &valueLength)
	if status != C.errSecSuccess {
		return "", fmt.Errorf("keychain resolve status %d", int32(status))
	}
	defer C.free(unsafe.Pointer(value))
	return string(C.GoBytes(unsafe.Pointer(value), C.int(valueLength))), nil
}

func (securityFrameworkBackend) Delete(ctx context.Context, service, account string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	serviceValue := C.CString(service)
	accountValue := C.CString(account)
	defer C.free(unsafe.Pointer(serviceValue))
	defer C.free(unsafe.Pointer(accountValue))
	status := C.gum_keychain_delete(serviceValue, accountValue)
	if status != C.errSecSuccess {
		return fmt.Errorf("keychain delete status %d", int32(status))
	}
	return nil
}

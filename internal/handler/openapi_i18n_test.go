package handler

import "testing"

func TestEnrichSpecWithI18nKeysForAssetImport(t *testing.T) {
	responses := map[string]interface{}{
		"200": map[string]interface{}{"description": "Import completed"},
		"400": map[string]interface{}{"description": "Asset count or field validation failed"},
		"403": map[string]interface{}{"description": "Missing asset:write permission or access denied for specified project"},
		"500": map[string]interface{}{"description": "Import transaction failed"},
	}
	operation := map[string]interface{}{
		"tags":      []string{"Asset Management"},
		"summary":   "bulk import assets",
		"responses": responses,
	}
	spec := map[string]interface{}{
		"paths": map[string]interface{}{
			"/api/assets/import": map[string]interface{}{
				"post": operation,
			},
		},
	}

	enrichSpecWithI18nKeys(spec)

	tagKeys, ok := operation["x-i18n-tags"].([]string)
	if !ok || len(tagKeys) != 1 || tagKeys[0] != "assetManagement" {
		t.Fatalf("unexpected asset tag i18n keys: %#v", operation["x-i18n-tags"])
	}
	if got := operation["x-i18n-summary"]; got != "importAssets" {
		t.Fatalf("unexpected asset summary i18n key: %#v", got)
	}
	expectedResponseKeys := map[string]string{
		"200": "assetImportCompleted",
		"400": "assetImportValidationFailed",
		"403": "assetImportForbidden",
		"500": "assetImportTransactionFailed",
	}
	for status, want := range expectedResponseKeys {
		response := responses[status].(map[string]interface{})
		if got := response["x-i18n-description"]; got != want {
			t.Errorf("unexpected asset response i18n key for %s: got %#v, want %q", status, got, want)
		}
	}
}

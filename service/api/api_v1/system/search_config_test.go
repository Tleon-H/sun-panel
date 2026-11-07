package system

import "testing"

func TestPublicSearchConfigKeepsOnlyTheCurrentEngine(t *testing.T) {
	view := PublicSearchConfig(map[string]interface{}{
		"currentId":     "custom-example",
		"newWindowOpen": true,
		"searchEngineList": []interface{}{
			map[string]interface{}{"title": "隐藏", "url": "https://example.com/hidden?q=%s"},
		},
		"engines": []interface{}{
			map[string]interface{}{"id": "google", "title": "Google", "url": "https://www.google.com/search?q=%s", "builtin": true},
			map[string]interface{}{"id": "custom-example", "title": "示例搜索", "url": "https://example.com/search?q=%s", "builtin": false},
		},
	})

	engines, _ := view["engines"].([]interface{})
	if len(engines) != 1 {
		t.Fatalf("public engines = %#v", view["engines"])
	}
	engine := engines[0].(map[string]interface{})
	if engine["url"] != "https://example.com/search?q=%s" {
		t.Fatalf("url = %#v", engine["url"])
	}
	if _, ok := view["searchEngineList"]; ok {
		t.Fatal("public view included the full custom list")
	}
	if view["newWindowOpen"] != true {
		t.Fatal("new window flag was dropped")
	}
}

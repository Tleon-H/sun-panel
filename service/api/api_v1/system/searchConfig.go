package system

// Search module settings belong to one account.
// A public visitor receives only the engine that account currently uses.
const searchModuleConfigName = "module-deskModuleSearchBox"

func PublicSearchConfig(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	out := map[string]interface{}{}
	if open, ok := value["newWindowOpen"]; ok {
		out["newWindowOpen"] = open
	}

	currentID, _ := value["currentId"].(string)
	var chosen map[string]interface{}
	if list, ok := value["engines"].([]interface{}); ok {
		for _, item := range list {
			row, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := row["id"].(string)
			if currentID != "" && id == currentID {
				chosen = row
				break
			}
		}
	}

	if chosen == nil {
		if current, ok := value["currentSearchEngine"].(map[string]interface{}); ok {
			out["currentSearchEngine"] = publicEngine(current)
		}
		return out
	}

	out["currentId"] = chosen["id"]
	out["engines"] = []interface{}{publicEngine(chosen)}
	out["currentSearchEngine"] = map[string]interface{}{
		"title":   chosen["title"],
		"url":     chosen["url"],
		"iconSrc": "",
	}
	return out
}

func publicEngine(row map[string]interface{}) map[string]interface{} {
	engine := map[string]interface{}{
		"title": row["title"],
		"url":   row["url"],
	}
	if id, ok := row["id"]; ok {
		engine["id"] = id
	}
	if builtin, ok := row["builtin"]; ok {
		engine["builtin"] = builtin
	}
	if _, ok := engine["id"]; !ok {
		engine["iconSrc"] = ""
	}
	return engine
}

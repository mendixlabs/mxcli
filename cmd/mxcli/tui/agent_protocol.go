package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AgentRequest is a JSON command from an external agent (e.g. Claude).
type AgentRequest struct {
	ID     int    `json:"id"`
	Action string `json:"action"`           // "exec", "check", "state", "navigate", "delete", "create_module", "format", "describe", "list"
	MDL    string `json:"mdl,omitempty"`    // for "exec", "format"
	Target string `json:"target,omitempty"` // for "navigate", "delete", "describe", "list" (e.g. "entity:Module.Entity")
	Name   string `json:"name,omitempty"`   // for "create_module"
}

// Validate checks that the request has all required fields for its action.
func (r AgentRequest) Validate() error {
	if r.ID == 0 {
		return fmt.Errorf("missing request id")
	}
	switch r.Action {
	case "exec":
		if r.MDL == "" {
			return fmt.Errorf("exec action requires mdl field")
		}
	case "format":
		if r.MDL == "" {
			return fmt.Errorf("format action requires mdl field")
		}
	case "check", "state":
		// no extra fields needed
	case "navigate", "delete", "describe":
		if r.Target == "" {
			return fmt.Errorf("%s action requires target field", r.Action)
		}
	case "list":
		if r.Target == "" {
			return fmt.Errorf("list action requires target field")
		}
	case "create_module":
		if r.Name == "" {
			return fmt.Errorf("create_module action requires name field")
		}
	default:
		return fmt.Errorf("unknown action: %q", r.Action)
	}
	return nil
}

// AgentResponse is the JSON response sent back to the agent.
type AgentResponse struct {
	ID      int             `json:"id"`
	OK      bool            `json:"ok"`
	Result  string          `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
	Mode    string          `json:"mode,omitempty"`    // e.g. "overlay:exec-result"
	Changes json.RawMessage `json:"changes,omitempty"` // structured changes from exec
}

// parseTarget splits "type:qualified.name" into (type, qualifiedName).
func parseTarget(target string) (string, string) {
	if idx := strings.Index(target, ":"); idx >= 0 {
		return target[:idx], target[idx+1:]
	}
	return target, ""
}

// buildAgentDescribeCmd returns the MDL DESCRIBE command for an agent target like "entity:Module.Entity".
// It delegates to buildDescribeCmd (preview.go) which handles multi-word types.
func buildAgentDescribeCmd(target string) (string, error) {
	nodeType, qname := parseTarget(target)
	if qname == "" {
		return "", fmt.Errorf("describe target must be type:QualifiedName (e.g. entity:Module.Entity)")
	}
	cmd := buildDescribeCmd(nodeType, qname)
	if cmd == "" {
		return "", fmt.Errorf("unsupported describe type: %q", nodeType)
	}
	return cmd, nil
}

// listKeywords maps lowercase node type names to their MDL SHOW keyword(s).
var listKeywords = map[string]string{
	"entities":         "ENTITIES",
	"associations":     "ASSOCIATIONS",
	"enumerations":     "ENUMERATIONS",
	"constants":        "CONSTANTS",
	"microflows":       "MICROFLOWS",
	"nanoflows":        "NANOFLOWS",
	"pages":            "PAGES",
	"snippets":         "SNIPPETS",
	"layouts":          "LAYOUTS",
	"workflows":        "WORKFLOWS",
	"modules":          "MODULES",
	"imagecollections": "IMAGE COLLECTIONS",
	"javaactions":      "JAVA ACTIONS",
}

// buildListCmd returns the MDL SHOW command for a target like "entities" or "entities:Module".
func buildListCmd(target string) (string, error) {
	nodeType, scope := parseTarget(target)
	keyword, ok := listKeywords[strings.ToLower(nodeType)]
	if !ok {
		return "", fmt.Errorf("unsupported list type: %q", nodeType)
	}
	cmd := "SHOW " + keyword
	if scope != "" {
		cmd += " IN " + scope
	}
	return cmd, nil
}

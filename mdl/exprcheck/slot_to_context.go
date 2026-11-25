// SPDX-License-Identifier: Apache-2.0

package exprcheck

func SlotToContext(slotPath string) string {
	if v, ok := slotContext[slotPath]; ok {
		return v
	}
	return "expression in microflow body"
}

var slotContext = map[string]string{
	"IfStmt.Condition":            "IF condition",
	"WhileStmt.Condition":         "WHILE condition",
	"ListOperation.Condition":     "FIND/FILTER predicate",
	"ChangeItem.Value":            "field of CHANGE",
	"CreateItem.Value":            "field of CREATE",
	"ReturnStmt.Value":            "RETURN value",
	"RetrieveStmt.LimitExpr":      "LIMIT clause",
	"RetrieveStmt.OffsetExpr":     "OFFSET clause",
	"LogStmt.Message":             "LOG message",
	"MfSetStmt.Value":             "right-hand side of SET",
	"DeclareStmt.InitialValue":    "initial value of DECLARE",
	"CallArgument.Value":          "argument of CALL",
	"SendEmailStmt.Address":       "SEND EMAIL address",
	"SendEmailStmt.Host":          "SEND EMAIL host",
	"SendEmailStmt.Port":          "SEND EMAIL port",
	"SendEmailStmt.Credential":    "SEND EMAIL user or password",
	"SendEmailStmt.TemplateParam": "SEND EMAIL template parameter",
	"MeterStmt.Name":              "metric name",
	"MeterStmt.CounterValue":      "INCREMENT COUNTER value",
	"MeterStmt.GaugeValue":        "SET GAUGE value",
	"MeterStmt.Tag":               "metric tag key or value",
}

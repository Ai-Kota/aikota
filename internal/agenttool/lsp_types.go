// Package agenttool implements aikota's built-in tools.
// This file defines LSP protocol types used by the LSP tool set.
package agenttool

// LspPosition is a line/column position in a text document (0-based, per LSP spec).
type LspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// LspLocation is a precise location in a document.
type LspLocation struct {
	Uri    string     `json:"uri"`
	Range  LspRange   `json:"range"`
}

// LspRange is a range of characters in a document.
type LspRange struct {
	Start LspPosition `json:"start"`
	End   LspPosition `json:"end"`
}

// LspSymbolKind is the kind of a symbol (class, function, variable, etc.).
type LspSymbolKind int

const (
	LspSymbolFile        LspSymbolKind = 1
	LspSymbolModule      LspSymbolKind = 2
	LspSymbolNamespace   LspSymbolKind = 3
	LspSymbolPackage     LspSymbolKind = 4
	LspSymbolClass       LspSymbolKind = 5
	LspSymbolMethod      LspSymbolKind = 6
	LspSymbolProperty    LspSymbolKind = 7
	LspSymbolField       LspSymbolKind = 8
	LspSymbolConstructor LspSymbolKind = 9
	LspSymbolEnum        LspSymbolKind = 10
	LspSymbolInterface   LspSymbolKind = 11
	LspSymbolFunction    LspSymbolKind = 12
	LspSymbolVariable    LspSymbolKind = 13
	LspSymbolConstant    LspSymbolKind = 14
	LspSymbolString      LspSymbolKind = 15
	LspSymbolNumber      LspSymbolKind = 16
	LspSymbolBoolean     LspSymbolKind = 17
	LspSymbolArray       LspSymbolKind = 18
	LspSymbolObject      LspSymbolKind = 19
	LspSymbolKey         LspSymbolKind = 20
	LspSymbolNull        LspSymbolKind = 21
	LspSymbolEnumMember  LspSymbolKind = 22
	LspSymbolStruct      LspSymbolKind = 23
	LspSymbolEvent       LspSymbolKind = 24
	LspSymbolOperator    LspSymbolKind = 25
	LspSymbolTypeParameter LspSymbolKind = 26
)

// LspSymbolKindName returns a human-readable name for a symbol kind.
func LspSymbolKindName(k LspSymbolKind) string {
	switch k {
	case LspSymbolFile:
		return "File"
	case LspSymbolModule:
		return "Module"
	case LspSymbolNamespace:
		return "Namespace"
	case LspSymbolPackage:
		return "Package"
	case LspSymbolClass:
		return "Class"
	case LspSymbolMethod:
		return "Method"
	case LspSymbolProperty:
		return "Property"
	case LspSymbolField:
		return "Field"
	case LspSymbolConstructor:
		return "Constructor"
	case LspSymbolEnum:
		return "Enum"
	case LspSymbolInterface:
		return "Interface"
	case LspSymbolFunction:
		return "Function"
	case LspSymbolVariable:
		return "Variable"
	case LspSymbolConstant:
		return "Constant"
	case LspSymbolString:
		return "String"
	case LspSymbolNumber:
		return "Number"
	case LspSymbolBoolean:
		return "Boolean"
	case LspSymbolArray:
		return "Array"
	case LspSymbolObject:
		return "Object"
	case LspSymbolKey:
		return "Key"
	case LspSymbolNull:
		return "Null"
	case LspSymbolEnumMember:
		return "Enum Member"
	case LspSymbolStruct:
		return "Struct"
	case LspSymbolEvent:
		return "Event"
	case LspSymbolOperator:
		return "Operator"
	case LspSymbolTypeParameter:
		return "Type Parameter"
	default:
		return "Unknown"
	}
}

// LspMarkedString represents a string that may have a language associated with it.
type LspMarkedString struct {
	Language string `json:"language,omitempty"`
	Value    string `json:"value"`
}

// LspHover represents hover information for a symbol.
type LspHover struct {
	Contents []LspMarkedString `json:"contents"`
	Range    *LspRange         `json:"range,omitempty"`
}

// LspDocumentation is formatted documentation for a symbol.
type LspDocumentation struct {
	Kind     string `json:"kind,omitempty"` // "plaintext" or "markdown"
	Value    string `json:"value"`
}

// LspSignatureInformation represents a function signature.
type LspSignatureInformation struct {
	Label          string              `json:"label"`
	Documentation  *LspDocumentation   `json:"documentation,omitempty"`
	Parameters     []LspParameterInformation `json:"parameters,omitempty"`
	ActiveParameter *int             `json:"activeParameter,omitempty"`
}

// LspParameterInformation represents a function parameter.
type LspParameterInformation struct {
	Label          string `json:"label"` // either (start, end) or label string
	Documentation  *LspDocumentation `json:"documentation,omitempty"`
}

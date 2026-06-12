package minitui

import "strings"

// ── ANSI token colors ─────────────────────────────────────────────

const (
	hlKeyword  = "\x1b[33m" // yellow — if, func, return …
	hlType     = "\x1b[36m" // cyan  — string, int, bool …
	hlString   = "\x1b[32m" // green — "hello", 'c'
	hlComment  = "\x1b[90m" // gray  — // comment, # comment
	hlNumber   = "\x1b[35m" // purple — 42, 3.14
	hlReset    = "\x1b[39m" // reset fg only (keep dim / bg)
)

// ── language definitions ──────────────────────────────────────────

type langDef struct {
	keywords   []string
	types      []string
	lineComment string // e.g. "//" or "#"
	blockOpen  string // e.g. "/*" (empty if not supported)
	blockClose string
}

var langTable = map[string]langDef{
	"go": {keywords: kwGo, types: tyGo, lineComment: "//", blockOpen: "/*", blockClose: "*/"},
	"golang": {keywords: kwGo, types: tyGo, lineComment: "//", blockOpen: "/*", blockClose: "*/"},

	"python": {keywords: kwPy, types: tyPy, lineComment: "#"},
	"py":     {keywords: kwPy, types: tyPy, lineComment: "#"},

	"javascript": {keywords: kwJS, types: tyJS, lineComment: "//", blockOpen: "/*", blockClose: "*/"},
	"js":         {keywords: kwJS, types: tyJS, lineComment: "//", blockOpen: "/*", blockClose: "*/"},
	"typescript": {keywords: kwJS, types: tyJS, lineComment: "//", blockOpen: "/*", blockClose: "*/"},
	"ts":         {keywords: kwJS, types: tyJS, lineComment: "//", blockOpen: "/*", blockClose: "*/"},

	"rust": {keywords: kwRust, types: tyRust, lineComment: "//", blockOpen: "/*", blockClose: "*/"},
	"rs":   {keywords: kwRust, types: tyRust, lineComment: "//", blockOpen: "/*", blockClose: "*/"},

	"bash": {keywords: kwBash, types: nil, lineComment: "#"},
	"sh":   {keywords: kwBash, types: nil, lineComment: "#"},
	"shell": {keywords: kwBash, types: nil, lineComment: "#"},
	"zsh":  {keywords: kwBash, types: nil, lineComment: "#"},

	"sql": {keywords: kwSQL, types: nil, lineComment: "--"},
}

var kwGo   = []string{"break","case","chan","const","continue","default","defer","else","fallthrough","for","func","go","goto","if","import","interface","map","package","range","return","select","struct","switch","type","var"}
var tyGo   = []string{"bool","byte","complex64","complex128","error","float32","float64","int","int8","int16","int32","int64","rune","string","uint","uint8","uint16","uint32","uint64","uintptr","nil","true","false","iota"}

var kwPy   = []string{"and","as","assert","async","await","break","class","continue","def","del","elif","else","except","finally","for","from","global","if","import","in","is","lambda","nonlocal","not","or","pass","raise","return","try","while","with","yield"}
var tyPy   = []string{"True","False","None","self","cls","int","float","str","bool","list","dict","tuple","set","bytes","bytearray"}

var kwJS   = []string{"async","await","break","case","catch","class","const","continue","debugger","default","delete","do","else","enum","export","extends","finally","for","function","if","import","in","instanceof","let","new","of","return","super","switch","this","throw","try","typeof","var","void","while","with","yield"}
var tyJS   = []string{"true","false","null","undefined","NaN","Infinity","number","string","boolean","object","symbol","bigint"}

var kwRust = []string{"as","async","await","break","const","continue","crate","dyn","else","enum","extern","false","fn","for","if","impl","in","let","loop","match","mod","move","mut","pub","ref","return","self","static","struct","super","trait","true","type","unsafe","use","where","while"}
var tyRust = []string{"bool","char","f32","f64","i8","i16","i32","i64","i128","isize","str","String","u8","u16","u32","u64","u128","usize","Vec","Option","Result","Some","None","Ok","Err","Box","Rc","Arc","Cell","RefCell","HashMap","HashSet","BTreeMap"}

var kwBash = []string{"alias","bg","bind","break","builtin","caller","case","cd","command","compgen","complete","continue","declare","dirs","disown","do","done","echo","elif","else","enable","esac","eval","exec","exit","export","false","fc","fg","fi","for","function","getopts","hash","help","history","if","in","jobs","kill","let","local","logout","mapfile","popd","printf","pushd","pwd","read","readarray","return","select","set","shift","shopt","source","suspend","test","then","time","times","trap","true","type","typeset","ulimit","umask","unalias","unset","until","variables","wait","while"}
var kwSQL  = []string{"select","from","where","and","or","not","in","is","null","like","between","join","inner","outer","left","right","full","on","as","into","insert","values","update","set","delete","create","alter","drop","table","index","view","distinct","group","by","order","asc","desc","having","limit","offset","union","all","case","when","then","else","end","exists","count","sum","avg","min","max","coalesce","cast","primary","key","foreign","references","constraint","default","check","unique","if","begin","commit","rollback","transaction"}

// ── highlight entry ───────────────────────────────────────────────

// highlightCodeBlock highlights a single line of source code.
// lang can be a fence info string like "go", "python", "js".
func highlightCodeBlock(line string, lang string) string {
	def, ok := langTable[strings.ToLower(strings.TrimSpace(lang))]
	if !ok || len(line) == 0 {
		return line
	}
	return highlightLine(line, def)
}

// ── line scanner ──────────────────────────────────────────────────

func highlightLine(s string, def langDef) string {
	var b strings.Builder
	b.Grow(len(s) + 48)

	i := 0
	for i < len(s) {
		// Line comment.
		if def.lineComment != "" && matchAt(s, i, def.lineComment) &&
			(i == 0 || s[i-1] != '"' && s[i-1] != '\'') {
			b.WriteString(hlComment)
			b.WriteString(s[i:])
			b.WriteString(hlReset)
			return b.String()
		}

		// Block comment open.
		if def.blockOpen != "" && i+len(def.blockOpen) <= len(s) {
			if matchAt(s, i, def.blockOpen) {
				b.WriteString(hlComment)
				b.WriteString(def.blockOpen)
				i += len(def.blockOpen)
				// Consume until block close or EOL.
				rest := s[i:]
				if idx := strings.Index(rest, def.blockClose); idx >= 0 {
					b.WriteString(rest[:idx+len(def.blockClose)])
					i += idx + len(def.blockClose)
				} else {
					b.WriteString(rest)
					i = len(s)
				}
				b.WriteString(hlReset)
				continue
			}
		}

		// String.
		if s[i] == '"' || s[i] == '\'' {
			quote := s[i]
			b.WriteString(hlString)
			b.WriteByte(quote)
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					b.WriteByte(s[i])
					b.WriteByte(s[i+1])
					i += 2
					continue
				}
				if s[i] == quote {
					b.WriteByte(s[i])
					i++
					break
				}
				b.WriteByte(s[i])
				i++
			}
			b.WriteString(hlReset)
			continue
		}

		// Number.
		if isDigit(s[i]) {
			start := i
			for i < len(s) && (isDigit(s[i]) || s[i] == '.' || s[i] == '_' || s[i] == 'x' || s[i] == 'X' || (s[i] >= 'a' && s[i] <= 'f') || (s[i] >= 'A' && s[i] <= 'F')) {
				i++
			}
			b.WriteString(hlNumber)
			b.WriteString(s[start:i])
			b.WriteString(hlReset)
			continue
		}

		// Word (identifier / keyword / type).
		if isWordStart(s[i]) {
			start := i
			for i < len(s) && isWordPart(s[i]) {
				i++
			}
			word := s[start:i]
			if contains(def.keywords, word) {
				b.WriteString(hlKeyword)
				b.WriteString(word)
				b.WriteString(hlReset)
			} else if contains(def.types, word) {
				b.WriteString(hlType)
				b.WriteString(word)
				b.WriteString(hlReset)
			} else {
				b.WriteString(word)
			}
			continue
		}

		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// ── integration helper ──────────────────────────────────────────

// extractLang returns the language name from a code fence line,
// e.g. "```go" → "go", "```python" → "python".
func extractLang(fence string) string {
	s := strings.TrimLeft(fence, "`~")
	return strings.TrimSpace(s)
}

func matchAt(s string, pos int, sub string) bool {
	return pos+len(sub) <= len(s) && s[pos:pos+len(sub)] == sub
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isWordStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isWordPart(c byte) bool {
	return isWordStart(c) || isDigit(c)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

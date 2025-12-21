package chat

import (
	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/lexers"
)

var mlldLexer = lexers.Register(chroma.MustNewLexer(
	&chroma.Config{
		Name:      "MLLD",
		Aliases:   []string{"mlld"},
		Filenames: []string{"*.mlld"},
		MimeTypes: []string{"text/x-mlld"},
		DotAll:    true,
		EnsureNL:  true,
	},
	chroma.Rules{
		"root": {
			{`\s+`, chroma.Text, nil},
			{`//[^\n]*`, chroma.CommentSingle, nil},
			{`#[^\n]*`, chroma.CommentSingle, nil},
			{`/\*.*?\*/`, chroma.CommentMultiline, nil},
			{`"(\\\\|\\"|[^"])*"`, chroma.LiteralStringDouble, nil},
			{`'(\\\\|\\'|[^'])*'`, chroma.LiteralStringSingle, nil},
			{`\b(true|false|null)\b`, chroma.KeywordConstant, nil},
			{`\b(let|set|if|else|for|in|return|fn|func|include|import|use|when|then|case|default)\b`, chroma.Keyword, nil},
			{`\b\d+(\.\d+)?\b`, chroma.LiteralNumber, nil},
			{`[{}()[\],.;]`, chroma.Punctuation, nil},
			{`[-+*/=<>!:%&|^]+`, chroma.Operator, nil},
			{`[A-Za-z_][A-Za-z0-9_]*`, chroma.Name, nil},
		},
	},
))

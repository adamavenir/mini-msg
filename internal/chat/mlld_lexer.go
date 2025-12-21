package chat

import (
	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/lexers"
)

// Stub lexer until the updated TextMate grammar is ported.
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
			{`[\s\S]+`, chroma.Text, nil},
		},
	},
))

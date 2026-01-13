import SwiftUI

struct CodeBlockView: View {
    let code: String
    let language: String?

    @State private var isCopied = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let lang = language, !lang.isEmpty {
                HStack {
                    Text(lang)
                        .font(.caption)
                        .foregroundStyle(.secondary)

                    Spacer()

                    Button(action: copyCode) {
                        HStack(spacing: 4) {
                            Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                            Text(isCopied ? "Copied" : "Copy")
                        }
                        .font(.caption)
                    }
                    .buttonStyle(.plain)
                    .foregroundStyle(.secondary)
                }
                .padding(.horizontal, FraySpacing.sm)
                .padding(.vertical, FraySpacing.xs)
                .background(Color.secondary.opacity(0.1))
            }

            ScrollView(.horizontal, showsIndicators: false) {
                Text(highlightedCode)
                    .font(FrayTypography.codeBlock)
                    .textSelection(.enabled)
                    .padding(FraySpacing.sm)
            }
        }
        .background(FrayColors.tertiaryBackground)
        .clipShape(RoundedRectangle(cornerRadius: FraySpacing.smallCornerRadius))
    }

    private var highlightedCode: AttributedString {
        var attributedString = AttributedString(code)

        guard let lang = language?.lowercased() else {
            return attributedString
        }

        let patterns = syntaxPatterns(for: lang)

        for (pattern, color) in patterns {
            guard let regex = try? NSRegularExpression(pattern: pattern, options: []) else {
                continue
            }

            let nsRange = NSRange(code.startIndex..., in: code)
            let matches = regex.matches(in: code, options: [], range: nsRange)

            for match in matches {
                guard let range = Range(match.range, in: code),
                      let attrRange = Range(range, in: attributedString) else {
                    continue
                }
                attributedString[attrRange].foregroundColor = color
            }
        }

        return attributedString
    }

    private func syntaxPatterns(for language: String) -> [(String, Color)] {
        let keywords: Color = .purple
        let strings: Color = .red
        let comments: Color = .gray
        let numbers: Color = .cyan
        let types: Color = .blue
        let functions: Color = .yellow

        switch language {
        case "swift":
            return [
                (#"//.*$"#, comments),
                (#"/\*[\s\S]*?\*/"#, comments),
                (#"\"(?:[^\"\\]|\\.)*\""#, strings),
                (#"\b(func|var|let|if|else|for|while|return|import|struct|class|enum|protocol|extension|guard|switch|case|default|break|continue|throw|try|catch|async|await)\b"#, keywords),
                (#"\b(String|Int|Bool|Double|Float|Array|Dictionary|Optional|Any|Void)\b"#, types),
                (#"\b\d+\.?\d*\b"#, numbers),
            ]
        case "go", "golang":
            return [
                (#"//.*$"#, comments),
                (#"/\*[\s\S]*?\*/"#, comments),
                (#"\"(?:[^\"\\]|\\.)*\""#, strings),
                (#"`[^`]*`"#, strings),
                (#"\b(func|var|const|if|else|for|range|return|import|package|struct|interface|type|map|chan|go|defer|select|switch|case|default|break|continue)\b"#, keywords),
                (#"\b(string|int|int64|bool|float64|error|byte|rune)\b"#, types),
                (#"\b\d+\.?\d*\b"#, numbers),
            ]
        case "javascript", "js", "typescript", "ts":
            return [
                (#"//.*$"#, comments),
                (#"/\*[\s\S]*?\*/"#, comments),
                (#"\"(?:[^\"\\]|\\.)*\""#, strings),
                (#"'(?:[^'\\]|\\.)*'"#, strings),
                (#"`(?:[^`\\]|\\.)*`"#, strings),
                (#"\b(function|const|let|var|if|else|for|while|return|import|export|from|class|extends|async|await|try|catch|throw|new|this)\b"#, keywords),
                (#"\b(string|number|boolean|object|Array|Promise|void)\b"#, types),
                (#"\b\d+\.?\d*\b"#, numbers),
            ]
        case "python", "py":
            return [
                (#"#.*$"#, comments),
                (#"\"\"\"[\s\S]*?\"\"\""#, strings),
                (#"'''[\s\S]*?'''"#, strings),
                (#"\"(?:[^\"\\]|\\.)*\""#, strings),
                (#"'(?:[^'\\]|\\.)*'"#, strings),
                (#"\b(def|class|if|elif|else|for|while|return|import|from|as|try|except|finally|raise|with|async|await|lambda|yield|pass|break|continue)\b"#, keywords),
                (#"\b(str|int|float|bool|list|dict|tuple|set|None|True|False)\b"#, types),
                (#"\b\d+\.?\d*\b"#, numbers),
            ]
        case "bash", "sh", "shell":
            return [
                (#"#.*$"#, comments),
                (#"\"(?:[^\"\\]|\\.)*\""#, strings),
                (#"'[^']*'"#, strings),
                (#"\b(if|then|else|elif|fi|for|while|do|done|case|esac|function|return|exit|echo|export|source)\b"#, keywords),
                (#"\$\{?[a-zA-Z_][a-zA-Z0-9_]*\}?"#, types),
                (#"\b\d+\b"#, numbers),
            ]
        default:
            return [
                (#"//.*$"#, comments),
                (#"#.*$"#, comments),
                (#"\"(?:[^\"\\]|\\.)*\""#, strings),
                (#"'(?:[^'\\]|\\.)*'"#, strings),
                (#"\b\d+\.?\d*\b"#, numbers),
            ]
        }
    }

    private func copyCode() {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(code, forType: .string)
        isCopied = true

        DispatchQueue.main.asyncAfter(deadline: .now() + 2) {
            isCopied = false
        }
    }
}

#Preview {
    VStack(spacing: 16) {
        CodeBlockView(code: """
            func greet(name: String) -> String {
                let message = "Hello, \\(name)!"
                return message
            }
            """, language: "swift")

        CodeBlockView(code: """
            func main() {
                fmt.Println("Hello, World!")
            }
            """, language: "go")

        CodeBlockView(code: """
            const greet = async (name) => {
                return `Hello, ${name}!`;
            };
            """, language: "javascript")
    }
    .padding()
    .frame(width: 500)
}

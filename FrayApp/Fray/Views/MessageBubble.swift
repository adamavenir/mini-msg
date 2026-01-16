import SwiftUI
import AppKit

struct MessageBubble: View {
    let message: FrayMessage
    var onReply: (() -> Void)?
    var showHeader: Bool = true
    var parentMessage: FrayMessage?

    @State private var isHovering = false
    @State private var hoverWorkItem: DispatchWorkItem?
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        HStack(alignment: .top, spacing: FraySpacing.sm) {
            AgentAvatar(agentId: message.fromAgent)
            messageBody
        }
        .padding(FraySpacing.messagePadding)
        .background {
            RoundedRectangle(cornerRadius: FraySpacing.cornerRadius)
                .fill(isHovering ? FrayColors.secondaryBackground : .clear)
        }
        .onHover { hovering in
            hoverWorkItem?.cancel()
            if hovering {
                if reduceMotion {
                    isHovering = true
                } else {
                    withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) {
                        isHovering = true
                    }
                }
            } else {
                // Grace period before hiding hover elements
                let workItem = DispatchWorkItem {
                    if reduceMotion {
                        isHovering = false
                    } else {
                        withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) {
                            isHovering = false
                        }
                    }
                }
                hoverWorkItem = workItem
                DispatchQueue.main.asyncAfter(deadline: .now() + FraySpacing.hoverGracePeriod, execute: workItem)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Message from \(message.fromAgent)")
        .accessibilityValue(message.body)
        .accessibilityHint("Double-tap to reply")
        .accessibilityAction(named: "Reply") {
            onReply?()
        }
    }

    private var messageBody: some View {
        VStack(alignment: .leading, spacing: FraySpacing.xs) {
            if showHeader {
                MessageHeader(
                    message: message,
                    isHovering: isHovering,
                    onReply: onReply
                )
            }

            if let parent = parentMessage {
                ParentMessageQuote(parent: parent)
            }

            MessageContent(content: message.body)

            if !message.reactions.isEmpty {
                ReactionBar(reactions: message.reactions, onReact: { _ in }, isHovering: isHovering)
            }

            MessageFooter(message: message)
        }
    }
}

struct MessageHeader: View {
    let message: FrayMessage
    let isHovering: Bool
    var onReply: (() -> Void)?

    var body: some View {
        HStack(spacing: FraySpacing.sm) {
            Text("@\(message.fromAgent)")
                .font(FrayTypography.agentName)
                .foregroundStyle(FrayColors.colorForAgent(message.fromAgent))

            Text(FrayFormatters.relativeTimestamp(message.ts))
                .font(FrayTypography.timestamp)
                .foregroundStyle(.secondary)

            if message.edited == true {
                Text("(edited)")
                    .font(FrayTypography.caption)
                    .foregroundStyle(.tertiary)
            }

            Spacer()

            if isHovering {
                MessageActions(message: message, onReply: onReply)
            }
        }
    }
}

struct ParentMessageQuote: View {
    let parent: FrayMessage

    var body: some View {
        HStack(spacing: FraySpacing.xs) {
            Rectangle()
                .fill(FrayColors.colorForAgent(parent.fromAgent))
                .frame(width: 2)

            VStack(alignment: .leading, spacing: 2) {
                Text("@\(parent.fromAgent)")
                    .font(FrayTypography.caption)
                    .foregroundStyle(FrayColors.colorForAgent(parent.fromAgent))
                Text(parent.body)
                    .font(FrayTypography.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
        }
        .padding(FraySpacing.sm)
        .background(FrayColors.tertiaryBackground)
        .cornerRadius(FraySpacing.smallCornerRadius)
    }
}

struct MessageActions: View {
    let message: FrayMessage
    var onReply: (() -> Void)?

    var body: some View {
        HStack(spacing: FraySpacing.xs) {
            Button(action: { onReply?() }) {
                Image(systemName: "arrowshape.turn.up.left")
            }
            .buttonStyle(.borderless)
            .help("Reply")
            .accessibilityLabel("Reply to message")

            Button(action: { print("React") }) {
                Image(systemName: "face.smiling")
            }
            .buttonStyle(.borderless)
            .help("Add reaction")
            .accessibilityLabel("Add reaction")

            Button(action: { print("More") }) {
                Image(systemName: "ellipsis")
            }
            .buttonStyle(.borderless)
            .help("More actions")
            .accessibilityLabel("More actions")
        }
        .font(.caption)
        .foregroundStyle(.secondary)
    }
}

enum ContentSegment: Identifiable {
    case text(AttributedString)
    case codeBlock(language: String?, code: String)

    var id: String {
        switch self {
        case .text(let attr): return "text-\(attr.hashValue)"
        case .codeBlock(_, let code): return "code-\(code.hashValue)"
        }
    }
}

struct MessageContent: View {
    let content: String

    var body: some View {
        let segments = parseSegments(content)
        VStack(alignment: .leading, spacing: FraySpacing.sm) {
            ForEach(segments) { segment in
                segmentView(segment)
            }
        }
    }

    @ViewBuilder
    private func segmentView(_ segment: ContentSegment) -> some View {
        switch segment {
        case .text(let attr):
            MessageTextView(attributedText: attr)
        case .codeBlock(let language, let code):
            CodeBlockView(code: code, language: language)
        }
    }

    private func parseSegments(_ text: String) -> [ContentSegment] {
        var segments: [ContentSegment] = []
        let pattern = "```(\\w*)\\n([\\s\\S]*?)```"

        guard let regex = try? NSRegularExpression(pattern: pattern, options: []) else {
            return [.text(parseContent(text))]
        }

        var lastEnd = text.startIndex
        let nsRange = NSRange(text.startIndex..., in: text)
        let matches = regex.matches(in: text, options: [], range: nsRange)

        for match in matches {
            guard let fullRange = Range(match.range, in: text),
                  let langRange = Range(match.range(at: 1), in: text),
                  let codeRange = Range(match.range(at: 2), in: text) else {
                continue
            }

            // Add text before this code block
            if lastEnd < fullRange.lowerBound {
                let textBefore = String(text[lastEnd..<fullRange.lowerBound]).trimmingCharacters(in: .whitespacesAndNewlines)
                if !textBefore.isEmpty {
                    segments.append(.text(parseContent(textBefore)))
                }
            }

            // Add the code block
            let language = String(text[langRange])
            let code = String(text[codeRange]).trimmingCharacters(in: .newlines)
            segments.append(.codeBlock(language: language.isEmpty ? nil : language, code: code))

            lastEnd = fullRange.upperBound
        }

        // Add remaining text after last code block
        if lastEnd < text.endIndex {
            let remaining = String(text[lastEnd...]).trimmingCharacters(in: .whitespacesAndNewlines)
            if !remaining.isEmpty {
                segments.append(.text(parseContent(remaining)))
            }
        }

        // If no segments found (no code blocks), return the whole text
        if segments.isEmpty {
            segments.append(.text(parseContent(text)))
        }

        return segments
    }

    private func parseContent(_ text: String) -> AttributedString {
        var result: AttributedString
        do {
            var options = AttributedString.MarkdownParsingOptions()
            options.interpretedSyntax = .inlineOnlyPreservingWhitespace
            result = try AttributedString(markdown: text, options: options)
        } catch {
            result = AttributedString(text)
        }

        // Find and style #fray-id patterns (including .n suffix like #fray-abc.1)
        // Pattern: #prefix-id or #prefix-id.n (e.g. #fray-abc123 or #fray-abc123.1)
        let idPattern = try? Regex("#[a-z]+-[a-z0-9]+(?:\\.[a-z0-9]+)?")
        guard let pattern = idPattern else { return result }

        let plainText = String(result.characters)
        for match in plainText.matches(of: pattern) {
            let matchStr = String(plainText[match.range])
            if let attrRange = result.range(of: matchStr) {
                result[attrRange].font = Font.system(size: 15, weight: .bold)
                if let url = URL(string: "frayid://\(matchStr.dropFirst())") {
                    result[attrRange].link = url
                }
            }
        }

        return result
    }
}

struct MessageTextView: View {
    let attributedText: AttributedString

    var body: some View {
        Text(attributedText)
            .font(FrayTypography.messageBody)
            .lineSpacing(FrayTypography.messageLineSpacing)
            .textSelection(.enabled)
            .environment(\.openURL, OpenURLAction { url in
                if url.scheme == "frayid" {
                    let id = "#\(url.host ?? "")"
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(id, forType: .string)
                    return .handled
                }
                return .systemAction
            })
    }
}

struct ReactionBar: View {
    let reactions: [String: [ReactionEntry]]
    let onReact: (String) -> Void
    var isHovering: Bool = false

    var body: some View {
        HStack(spacing: FraySpacing.xs) {
            ForEach(Array(reactions.keys.sorted()), id: \.self) { emoji in
                ReactionPill(emoji: emoji, entries: reactions[emoji] ?? [])
                    .onTapGesture { onReact(emoji) }
            }

            Button(action: { }) {
                Image(systemName: "face.smiling")
                    .font(.caption)
            }
            .buttonStyle(.borderless)
            .opacity(isHovering ? 1 : 0)
        }
    }
}

struct ReactionPill: View {
    let emoji: String
    let entries: [ReactionEntry]

    var body: some View {
        HStack(spacing: FraySpacing.xs) {
            Text(emoji)
            Text("\(entries.count)")
                .font(FrayTypography.reactionCount)
                .foregroundStyle(.secondary)
        }
        .padding(.horizontal, FraySpacing.sm)
        .padding(.vertical, FraySpacing.xs)
        .background {
            Capsule()
                .fill(FrayColors.tertiaryBackground)
        }
    }
}

struct MessageFooter: View {
    let message: FrayMessage

    var body: some View {
        HStack(spacing: FraySpacing.sm) {
            CopyableIdText(id: message.id)

            if let sessionId = message.sessionId {
                Text("•")
                    .font(.caption2)
                    .foregroundStyle(.quaternary)
                // Display @agent#sessid (first 5 chars), copy full @agent#sessionId
                let shortSess = String(sessionId.prefix(5))
                let displayText = "@\(message.fromAgent)#\(shortSess)"
                let copyText = "@\(message.fromAgent)#\(sessionId)"
                CopyableIdText(id: displayText, copyValue: copyText)
            }

            Spacer()
        }
        .padding(.top, FraySpacing.xs)
    }
}

struct CopyableIdText: View {
    let id: String
    var copyValue: String?
    @State private var isCopied = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var valueToCopy: String {
        copyValue ?? id
    }

    var body: some View {
        Text(id)
            .font(.caption2.monospaced())
            .foregroundStyle(isCopied ? AnyShapeStyle(Color.accentColor) : AnyShapeStyle(.quaternary))
            .contentShape(Rectangle())
            .onTapGesture {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(valueToCopy, forType: .string)
                if reduceMotion {
                    isCopied = true
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.8) {
                        isCopied = false
                    }
                } else {
                    withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) {
                        isCopied = true
                    }
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.8) {
                        withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) {
                            isCopied = false
                        }
                    }
                }
            }
            .help("Click to copy: \(valueToCopy)")
    }
}

#Preview {
    MessageBubble(
        message: FrayMessage(
            id: "msg-test123",
            ts: Int64(Date().timeIntervalSince1970),
            channelId: nil,
            home: nil,
            fromAgent: "opus",
            sessionId: nil,
            body: "Hello world! This is a **test** message with `code`.",
            mentions: [],
            forkSessions: nil,
            reactions: ["👍": [ReactionEntry(agentId: "adam", reactedAt: 0)]],
            type: .agent,
            references: nil,
            surfaceMessage: nil,
            replyTo: nil,
            quoteMessageGuid: nil,
            editedAt: nil,
            edited: false,
            editCount: nil,
            archivedAt: nil
        )
    )
    .padding()
}

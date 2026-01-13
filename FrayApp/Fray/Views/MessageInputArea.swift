import SwiftUI
import AppKit

struct MessageInputArea: View {
    @Binding var text: String
    @Binding var replyTo: FrayMessage?
    let onSubmit: (String) -> Void

    @FocusState private var isFocused: Bool
    @State private var suggestions: [Suggestion] = []

    var body: some View {
        VStack(spacing: 0) {
            if let reply = replyTo {
                ReplyPreview(message: reply) {
                    withAnimation {
                        replyTo = nil
                    }
                }
            }

            HStack(alignment: .bottom, spacing: FraySpacing.sm) {
                FrayTextEditor(
                    text: $text,
                    isFocused: $isFocused,
                    onSubmit: handleSubmit
                )
                .frame(minHeight: 36, maxHeight: 120)

                Button(action: handleSubmit) {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.title2)
                }
                .buttonStyle(.borderless)
                .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                .keyboardShortcut(.return, modifiers: [.command])
            }
            .padding(FraySpacing.sm)
            .background {
                RoundedRectangle(cornerRadius: FraySpacing.cornerRadius)
                    .fill(.regularMaterial)
            }
        }
    }

    private func handleSubmit() {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        onSubmit(trimmed)
    }
}

struct ReplyPreview: View {
    let message: FrayMessage
    let onDismiss: () -> Void

    var body: some View {
        HStack(spacing: FraySpacing.xs) {
            Rectangle()
                .fill(FrayColors.colorForAgent(message.fromAgent))
                .frame(width: 2)

            Text("Replying to")
                .font(FrayTypography.caption)
                .foregroundStyle(.tertiary)

            Text("@\(message.fromAgent)")
                .font(FrayTypography.caption)
                .foregroundStyle(FrayColors.colorForAgent(message.fromAgent))

            Text(message.body.replacingOccurrences(of: "\n", with: " "))
                .font(FrayTypography.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.tail)

            Spacer(minLength: 4)

            Button(action: onDismiss) {
                Image(systemName: "xmark.circle.fill")
                    .font(.caption)
            }
            .buttonStyle(.borderless)
            .foregroundStyle(.tertiary)
        }
        .padding(.horizontal, FraySpacing.sm)
        .padding(.vertical, FraySpacing.xs)
    }
}

struct FrayTextEditor: NSViewRepresentable {
    @Binding var text: String
    var isFocused: FocusState<Bool>.Binding
    var onSubmit: () -> Void

    func makeNSView(context: Context) -> NSScrollView {
        let scrollView = NSScrollView()
        let textView = FrayNSTextView()

        textView.delegate = context.coordinator
        textView.isRichText = false
        textView.font = NSFont.systemFont(ofSize: NSFont.systemFontSize)
        textView.textContainerInset = NSSize(width: 8, height: 8)
        textView.isVerticallyResizable = true
        textView.isHorizontallyResizable = false
        textView.autoresizingMask = [.width]
        textView.textContainer?.widthTracksTextView = true
        textView.backgroundColor = .clear

        scrollView.documentView = textView
        scrollView.hasVerticalScroller = false
        scrollView.hasHorizontalScroller = false
        scrollView.autohidesScrollers = true
        scrollView.borderType = .noBorder
        scrollView.drawsBackground = false

        context.coordinator.textView = textView
        context.coordinator.onSubmit = onSubmit

        return scrollView
    }

    func updateNSView(_ scrollView: NSScrollView, context: Context) {
        guard let textView = scrollView.documentView as? NSTextView else { return }

        if textView.string != text {
            textView.string = text
        }

        if isFocused.wrappedValue && textView.window?.firstResponder != textView {
            textView.window?.makeFirstResponder(textView)
        }
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(text: $text, isFocused: isFocused)
    }

    class Coordinator: NSObject, NSTextViewDelegate {
        @Binding var text: String
        var isFocused: FocusState<Bool>.Binding
        var textView: NSTextView?
        var onSubmit: (() -> Void)?

        init(text: Binding<String>, isFocused: FocusState<Bool>.Binding) {
            _text = text
            self.isFocused = isFocused
        }

        func textDidChange(_ notification: Notification) {
            guard let textView = notification.object as? NSTextView else { return }
            text = textView.string
        }

        func textDidBeginEditing(_ notification: Notification) {
            isFocused.wrappedValue = true
        }

        func textDidEndEditing(_ notification: Notification) {
            isFocused.wrappedValue = false
        }
    }
}

class FrayNSTextView: NSTextView {
    var onSubmit: (() -> Void)?

    override func keyDown(with event: NSEvent) {
        let modifiers = NSApp.currentEvent?.modifierFlags ?? event.modifierFlags

        if event.keyCode == 36 { // Enter key
            // Shift+Enter or Option+Enter inserts newline
            if modifiers.contains(.shift) || modifiers.contains(.option) {
                insertNewline(nil)
            } else {
                // Plain Enter submits
                if let coordinator = delegate as? FrayTextEditor.Coordinator {
                    coordinator.onSubmit?()
                }
            }
            return
        }

        super.keyDown(with: event)
    }
}

enum Suggestion: Identifiable {
    case agent(FrayAgent)
    case thread(FrayThread)
    case command(String, String)

    var id: String {
        switch self {
        case .agent(let agent): return "agent-\(agent.guid)"
        case .thread(let thread): return "thread-\(thread.guid)"
        case .command(let name, _): return "cmd-\(name)"
        }
    }
}

struct SuggestionsPopup: View {
    let suggestions: [Suggestion]
    let onSelect: (Suggestion) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(suggestions) { suggestion in
                SuggestionRow(suggestion: suggestion)
                    .onTapGesture { onSelect(suggestion) }
            }
        }
        .background {
            RoundedRectangle(cornerRadius: FraySpacing.cornerRadius)
                .fill(.regularMaterial)
        }
    }
}

struct SuggestionRow: View {
    let suggestion: Suggestion

    var body: some View {
        HStack(spacing: FraySpacing.sm) {
            switch suggestion {
            case .agent(let agent):
                AgentAvatar(agentId: agent.agentId, size: 20)
                Text("@\(agent.agentId)")
                    .font(FrayTypography.agentName)

            case .thread(let thread):
                Image(systemName: "number")
                    .foregroundStyle(.secondary)
                Text(thread.name)

            case .command(let name, let description):
                Image(systemName: "command")
                    .foregroundStyle(.secondary)
                Text("/\(name)")
                    .font(FrayTypography.agentName)
                Text(description)
                    .foregroundStyle(.secondary)
            }

            Spacer()
        }
        .padding(.horizontal, FraySpacing.sm)
        .padding(.vertical, FraySpacing.xs)
        .contentShape(Rectangle())
    }
}

#Preview {
    MessageInputArea(
        text: .constant("Hello"),
        replyTo: .constant(nil),
        onSubmit: { _ in }
    )
    .padding()
    .frame(width: 400)
}

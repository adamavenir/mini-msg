export interface FormattedMessage {
  id: string;
  projectName: string;
  type: 'user' | 'agent';
  sender: string;
  body: string;
  reply_to: string | null;
}

export interface ChatDisplay {
  renderMessage(msg: FormattedMessage): void;
  renderFullMessage(msg: FormattedMessage): void;
  showStatus(text: string): void;
  destroy(): void;
}

export interface ChatInput {
  start(): void;
  onMessage(callback: (text: string) => void): void;
  onQuit(callback: () => void): void;
  destroy(): void;
}

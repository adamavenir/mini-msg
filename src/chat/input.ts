import readline from 'readline';
import type { ChatInput } from './types.js';

export class ReadlineChatInput implements ChatInput {
  private rl: readline.Interface | null = null;
  private messageCallback: ((text: string) => void) | null = null;
  private quitCallback: (() => void) | null = null;
  private accumulator: string[] = [];

  start(): void {
    this.rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
      prompt: '', // No prompt prefix, text flush left
    });

    this.rl.setPrompt(''); // Ensure prompt is empty

    this.rl.on('line', (line: string) => {
      // Check if line ends with backslash (line continuation)
      const endsWithBackslash = line.endsWith('\\');

      if (endsWithBackslash) {
        // Remove the trailing backslash and accumulate
        this.accumulator.push(line.slice(0, -1));
      } else {
        // No backslash = this is the last line, submit
        if (this.accumulator.length > 0) {
          // We have accumulated lines, add this one and submit
          this.accumulator.push(line);
          const message = this.accumulator.join('\n');
          const lineCount = this.accumulator.length;
          this.accumulator = [];

          // Clear the accumulated input lines from the terminal
          for (let i = 0; i < lineCount; i++) {
            process.stdout.write('\x1b[1A'); // Move up one line
            process.stdout.write('\x1b[2K'); // Clear entire line
          }

          if (this.messageCallback) {
            this.messageCallback(message);
          }
        } else {
          // Single line message, submit immediately
          if (line.trim() !== '') {
            // Clear the line we just typed
            process.stdout.write('\x1b[1A');
            process.stdout.write('\x1b[2K');

            if (this.messageCallback) {
              this.messageCallback(line);
            }
          }
        }
      }
    });

    this.rl.on('close', () => {
      if (this.quitCallback) {
        this.quitCallback();
      }
    });

    this.rl.on('SIGINT', () => {
      if (this.quitCallback) {
        this.quitCallback();
      }
      this.destroy();
    });
  }

  onMessage(callback: (text: string) => void): void {
    this.messageCallback = callback;
  }

  onQuit(callback: () => void): void {
    this.quitCallback = callback;
  }

  destroy(): void {
    if (this.rl) {
      this.rl.close();
      this.rl = null;
    }
  }
}

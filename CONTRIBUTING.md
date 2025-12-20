# Contributing to mini-msg

Thanks for considering contributing to mini-msg!

## Development Setup

```bash
git clone https://github.com/adamavenir/mini-msg.git
cd mini-msg
npm install
npm run build
npm test
```

## Testing Your Changes

```bash
npm test          # Run test suite
npm run build     # Build the project
npm run dev       # Watch mode for development
```

Test your changes locally:

```bash
npm link          # Link globally for testing
mm init           # Test the CLI
```

## Code Style

- TypeScript strict mode enabled
- Keep functions focused and small
- Add tests for new features
- Use existing patterns in the codebase

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/your-feature`)
3. Make your changes
4. Add tests if applicable
5. Ensure tests pass (`npm test`)
6. Ensure build succeeds (`npm run build`)
7. Commit your changes (`git commit -m 'Add some feature'`)
8. Push to your fork (`git push origin feature/your-feature`)
9. Open a Pull Request

## Commit Messages

- Use clear, descriptive commit messages
- Start with a verb in present tense (Add, Fix, Update, etc.)
- Reference issues where applicable

## Reporting Bugs

Open an issue at https://github.com/adamavenir/mini-msg/issues with:
- Clear description of the problem
- Steps to reproduce
- Expected vs actual behavior
- Your environment (OS, Node version)

## Feature Requests

Feature requests are welcome! Open an issue describing:
- The use case
- Why it would be valuable
- Any implementation ideas

## Questions?

Feel free to open an issue for questions or discussion.

# Contributing to mini-msg

Thanks for considering contributing to mini-msg!

## Development Setup

```bash
git clone https://github.com/adamavenir/mini-msg.git
cd mini-msg
go test ./...
go build ./cmd/mm
```

## Testing Your Changes

```bash
go test ./...     # Run test suite
go build ./cmd/mm # Build the project
```

Test your changes locally:

```bash
go install ./cmd/mm # Install locally for testing
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
5. Ensure tests pass (`go test ./...`)
6. Ensure build succeeds (`go build ./cmd/mm`)
7. Commit your changes (`git commit -m 'Add some feature'`)
8. Push to your fork (`git push origin feature/your-feature`)
9. Open a Pull Request

## Release Flow

- Update `CHANGELOG.md` with the new version and changes.
- Keep `package.json` version in sync with the changelog.
- Merge to `main` to trigger the release workflow.
- The workflow tags `vX.Y.Z`, runs GoReleaser, updates the Homebrew formula, and publishes the npm package via trusted publishing.

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

package hooks

// safetyGuardScript is the Python script that blocks dangerous git commands
// when .fray/ has uncommitted changes.
const safetyGuardScript = `#!/usr/bin/env python3
"""
Fray Safety Guard - Protects .fray/ data from destructive git operations.

This hook blocks dangerous git commands when .fray/ has uncommitted changes,
preventing accidental loss of chat history and agent context.

Blocked commands:
- git stash (when .fray/ dirty) - could lose uncommitted fray data
- git checkout/restore <files> (when .fray/ dirty) - discards changes
- git reset --hard (when .fray/ dirty) - destroys uncommitted work
- git clean -f (always) - deletes untracked files
- rm -rf .fray or rm .fray/*.jsonl - protects source of truth
- git push --force to main/master - prevents history destruction

Install with: fray hook-install --safety
"""

import json
import re
import subprocess
import sys


def has_uncommitted_fray():
    """Check if .fray/ has uncommitted changes."""
    try:
        result = subprocess.run(
            ["git", "status", "--porcelain", ".fray/"],
            capture_output=True,
            text=True,
            timeout=5
        )
        return bool(result.stdout.strip())
    except Exception:
        return False


def get_uncommitted_fray_files():
    """Get list of uncommitted .fray/ files."""
    try:
        result = subprocess.run(
            ["git", "status", "--porcelain", ".fray/"],
            capture_output=True,
            text=True,
            timeout=5
        )
        return result.stdout.strip()
    except Exception:
        return ""


def deny(reason):
    """Return a denial response."""
    print(json.dumps({
        "hookSpecificOutput": {
            "permissionDecision": "deny",
            "permissionDecisionReason": reason
        }
    }))
    sys.exit(0)


def check_command(cmd):
    """Check if command should be blocked."""
    if not cmd or not isinstance(cmd, str):
        return

    # Normalize: handle /usr/bin/git, /bin/rm, etc.
    words = cmd.split()
    if not words:
        return

    # Get base command name
    base_cmd = os.path.basename(words[0])

    # === ALWAYS BLOCKED ===

    # Block rm -rf .fray or rm on .fray/*.jsonl
    if base_cmd == "rm":
        if ".fray" in cmd:
            if re.search(r'\.fray\b', cmd):
                deny(
                    "BLOCKED: Deleting .fray/ would destroy chat history.\n\n"
                    "The .fray/ directory contains your message history and agent data.\n"
                    "If you really need to remove it, do so manually outside of Claude Code."
                )

    # Block git clean -f (always dangerous for untracked files)
    if base_cmd == "git" and "clean" in cmd:
        # Allow dry-run: git clean -n, git clean --dry-run
        if "-n" in words or "--dry-run" in words:
            return
        # Block forced clean
        if "-f" in words or "--force" in words:
            deny(
                "BLOCKED: 'git clean -f' permanently deletes untracked files.\n\n"
                "This could delete .fray/ data or other important untracked files.\n"
                "Use 'git clean -n' to preview what would be deleted first."
            )

    # Block git push --force to main/master
    if base_cmd == "git" and "push" in cmd:
        has_force = "--force" in words or "-f" in words
        targets_main = "main" in words or "master" in words or "origin main" in cmd or "origin master" in cmd
        # Also catch push -f without explicit branch (pushes to current)
        if has_force and (targets_main or len([w for w in words if not w.startswith("-")]) <= 2):
            deny(
                "BLOCKED: Force pushing can destroy remote history.\n\n"
                "This is especially dangerous for main/master branches.\n"
                "If you need to force push, do so manually with explicit confirmation."
            )

    # === BLOCKED WHEN .fray/ IS DIRTY ===

    if not has_uncommitted_fray():
        return  # .fray/ is clean, allow command

    dirty_files = get_uncommitted_fray_files()
    fray_warning = (
        f"\n\nUncommitted .fray/ changes:\n{dirty_files}\n\n"
        "To proceed safely:\n"
        "  git add .fray/ && git commit -m 'fray: checkpoint'"
    )

    # Block git stash when .fray/ is dirty
    if base_cmd == "git" and "stash" in words:
        # Allow stash pop, stash apply, stash list, stash show
        safe_stash = any(w in words for w in ["pop", "apply", "list", "show", "branch"])
        if not safe_stash:
            deny(
                "BLOCKED: 'git stash' with uncommitted .fray/ changes.\n\n"
                "Stashing could lose your fray chat history if you forget to pop it "
                "or switch branches." + fray_warning
            )

    # Block git checkout <files> when .fray/ is dirty (but allow branch checkout)
    if base_cmd == "git" and "checkout" in words:
        # Allow: git checkout -b <branch>, git checkout <branch>
        # Block: git checkout -- <files>, git checkout <files>
        if "-b" in words or "--orphan" in words:
            return  # Creating new branch is safe
        if "--" in words:
            deny(
                "BLOCKED: 'git checkout --' discards uncommitted changes.\n\n"
                "This would permanently lose your .fray/ changes." + fray_warning
            )
        # Check if checking out files (not branches) - heuristic: has file-like args
        for word in words[2:]:  # Skip 'git checkout'
            if word.startswith("-"):
                continue
            if "." in word or "/" in word:
                deny(
                    "BLOCKED: 'git checkout <files>' with uncommitted .fray/ changes.\n\n"
                    "This could discard your fray data." + fray_warning
                )

    # Block git restore when .fray/ is dirty
    if base_cmd == "git" and "restore" in words:
        # Allow: git restore --staged (just unstages)
        if "--staged" in words and "--worktree" not in words:
            return
        deny(
            "BLOCKED: 'git restore' with uncommitted .fray/ changes.\n\n"
            "This could discard your fray data." + fray_warning
        )

    # Block git reset --hard when .fray/ is dirty
    if base_cmd == "git" and "reset" in words:
        if "--hard" in words or "--merge" in words:
            deny(
                "BLOCKED: 'git reset --hard' with uncommitted .fray/ changes.\n\n"
                "This would permanently destroy your uncommitted work." + fray_warning
            )


def main():
    try:
        input_data = sys.stdin.read()
        if not input_data.strip():
            return

        data = json.loads(input_data)
        tool_name = data.get("tool_name", "")

        if tool_name != "Bash":
            return

        tool_input = data.get("tool_input", {})
        if isinstance(tool_input, dict):
            command = tool_input.get("command", "")
        else:
            return

        check_command(command)

    except json.JSONDecodeError:
        pass
    except Exception:
        pass


if __name__ == "__main__":
    main()
`

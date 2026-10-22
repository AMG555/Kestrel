#!/usr/bin/env python3
"""
Patch validator and application engine for Kestrel.
Supports unified diff inspection, dry-run conflict checking, backup creation,
and safe remediation application across repositories.
"""

import sys
import os
import re
import json
import shutil
import tempfile
import subprocess
from typing import Dict, Any, List, Optional, Tuple

def parse_patch_summary(patch_text: str) -> Dict[str, Any]:
    files = []
    additions = 0
    deletions = 0
    
    current_file = None
    for line in patch_text.splitlines():
        if line.startswith("--- "):
            pass
        elif line.startswith("+++ "):
            # Strip prefixes like a/ b/ or leading slashes
            path_part = line[4:].strip()
            if path_part.startswith("b/"):
                path_part = path_part[2:]
            elif path_part.startswith("a/"):
                path_part = path_part[2:]
            current_file = path_part
            if current_file and current_file != "/dev/null" and current_file not in files:
                files.append(current_file)
        elif line.startswith("+") and not line.startswith("+++"):
            additions += 1
        elif line.startswith("-") and not line.startswith("---"):
            deletions += 1
            
    return {
        "files": files,
        "additions": additions,
        "deletions": deletions
    }

def try_git_apply(patch_text: str, directory: str, strip: int, check_only: bool = False, reverse: bool = False) -> Tuple[bool, str]:
    # Check if directory is inside a git repo
    try:
        cmd = ["git", "rev-parse", "--is-inside-work-tree"]
        res = subprocess.run(cmd, cwd=directory, capture_output=True, text=True, timeout=5)
        if res.returncode != 0:
            return False, "Not a git repository"
    except Exception as e:
        return False, f"Git unavailable: {str(e)}"

    # Write patch to a temporary file
    with tempfile.NamedTemporaryFile("w", delete=False, suffix=".patch", encoding="utf-8") as f:
        f.write(patch_text)
        temp_patch_path = f.name

    try:
        git_cmd = ["git", "apply", f"-p{strip}"]
        if check_only:
            git_cmd.append("--check")
        if reverse:
            git_cmd.append("--reverse")
        git_cmd.append(temp_patch_path)

        run_res = subprocess.run(git_cmd, cwd=directory, capture_output=True, text=True, timeout=10)
        if run_res.returncode == 0:
            return True, "Git apply executed successfully"
        else:
            err = (run_res.stderr or run_res.stdout or "Patch conflict or mismatch").strip()
            return False, err
    finally:
        if os.path.exists(temp_patch_path):
            try:
                os.remove(temp_patch_path)
            except Exception:
                pass

def apply_pure_python(patch_text: str, directory: str, strip: int, dry_run: bool = False, reverse: bool = False) -> Tuple[bool, List[str], str]:
    """Pure-python fallback patch implementation for unified diffs."""
    # Split by file headers
    file_diffs = re.split(r'(?m)(?=^--- )', patch_text)
    modified_files = []

    for file_diff in file_diffs:
        if not file_diff.strip():
            continue
            
        header_match = re.search(r'^--- (?:[ab]/)?([^\s\t\n]+)\n\+\+\+ (?:[ab]/)?([^\s\t\n]+)', file_diff, re.M)
        if not header_match:
            continue
            
        target_path_rel = header_match.group(2) if not reverse else header_match.group(1)
        if target_path_rel == "/dev/null":
            target_path_rel = header_match.group(1) if not reverse else header_match.group(2)

        # Handle strip
        parts = target_path_rel.replace('\\', '/').split('/')
        if strip > 0 and len(parts) > strip:
            target_path_rel = '/'.join(parts[strip:])
            
        target_file = os.path.join(directory, target_path_rel)
        if not os.path.exists(target_file):
            return False, [], f"Target file does not exist: {target_file}"

        try:
            with open(target_file, "r", encoding="utf-8", errors="replace") as f:
                original_lines = f.read().splitlines()
        except Exception as e:
            return False, [], f"Cannot read {target_file}: {e}"

        # Parse hunks
        hunk_regex = re.compile(r'^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$', re.M)
        hunk_splits = hunk_regex.split(file_diff)
        if len(hunk_splits) <= 1:
            continue

        # hunk_splits format: [prefix, old_start, old_len, new_start, new_len, title, hunk_body, ...]
        new_lines = list(original_lines)
        offset = 0

        i = 1
        while i < len(hunk_splits):
            old_start = int(hunk_splits[i])
            old_len = int(hunk_splits[i+1]) if hunk_splits[i+1] else 1
            new_start = int(hunk_splits[i+2])
            new_len = int(hunk_splits[i+3]) if hunk_splits[i+3] else 1
            hunk_body = hunk_splits[i+5]
            i += 6

            hunk_lines = hunk_body.splitlines()
            expected_old = []
            replacement = []

            for h_line in hunk_lines:
                if not h_line:
                    continue
                prefix = h_line[0]
                content = h_line[1:]
                if reverse:
                    if prefix == '+':
                        expected_old.append(content)
                    elif prefix == '-':
                        replacement.append(content)
                    elif prefix == ' ':
                        expected_old.append(content)
                        replacement.append(content)
                else:
                    if prefix == '-':
                        expected_old.append(content)
                    elif prefix == '+':
                        replacement.append(content)
                    elif prefix == ' ':
                        expected_old.append(content)
                        replacement.append(content)

            # Match context in original file
            pos = old_start - 1 + offset
            # Verify slice
            slice_end = pos + len(expected_old)
            if pos < 0 or slice_end > len(new_lines):
                return False, [], f"Hunk at line {old_start} exceeds file bounds in {target_path_rel}"

            actual_slice = new_lines[pos:slice_end]
            if actual_slice != expected_old:
                # Context mismatch
                return False, [], f"Conflict in {target_path_rel}: hunk at line {old_start} does not match file contents"

            if not dry_run:
                new_lines[pos:slice_end] = replacement
                offset += len(replacement) - len(expected_old)

        if not dry_run:
            # Backup original
            bak_path = target_file + ".kestrel.bak"
            shutil.copy2(target_file, bak_path)
            with open(target_file, "w", encoding="utf-8") as f:
                f.write("\n".join(new_lines) + "\n")
                
        modified_files.append(target_file)

    return True, modified_files, "Applied cleanly"

def execute_patch_op(
    operation: str,
    patch_text: str,
    directory: str = ".",
    strip: int = 1
) -> Dict[str, Any]:
    directory = os.path.abspath(directory)
    if not os.path.exists(directory):
        return {
            "status": "error",
            "message": f"Target directory does not exist: {directory}"
        }

    # If patch_text is a file path, load it
    if os.path.isfile(patch_text) or os.path.isfile(os.path.join(directory, patch_text)):
        path_to_open = patch_text if os.path.isfile(patch_text) else os.path.join(directory, patch_text)
        try:
            with open(path_to_open, "r", encoding="utf-8") as f:
                patch_text = f.read()
        except Exception as e:
            return {
                "status": "error",
                "message": f"Failed to read patch file {path_to_open}: {str(e)}"
            }

    summary = parse_patch_summary(patch_text)
    if not summary["files"]:
        return {
            "status": "error",
            "message": "Invalid patch format: no target files detected in diff header (expected '--- a/file' and '+++ b/file')."
        }

    op = operation.lower().strip()

    if op in ("check", "dry_run", "validate"):
        # 1. Try git apply --check first
        git_ok, git_msg = try_git_apply(patch_text, directory, strip, check_only=True)
        if git_ok:
            return {
                "status": "success",
                "clean": True,
                "operation": "check",
                "engine": "git",
                "files_affected": summary["files"],
                "additions": summary["additions"],
                "deletions": summary["deletions"],
                "message": f"Patch validated: applies cleanly to {len(summary['files'])} file(s) with zero conflicts."
            }
        
        # 2. Try pure-python dry run
        py_ok, py_files, py_msg = apply_pure_python(patch_text, directory, strip, dry_run=True)
        if py_ok:
            return {
                "status": "success",
                "clean": True,
                "operation": "check",
                "engine": "python",
                "files_affected": summary["files"],
                "additions": summary["additions"],
                "deletions": summary["deletions"],
                "message": f"Patch validated: applies cleanly to {len(summary['files'])} file(s)."
            }
        else:
            return {
                "status": "conflict",
                "clean": False,
                "operation": "check",
                "error": py_msg,
                "git_error": git_msg,
                "files_affected": summary["files"],
                "message": f"Patch cannot be applied cleanly: {py_msg}"
            }

    elif op in ("apply", "commit"):
        # Check dry run first
        git_ok, git_msg = try_git_apply(patch_text, directory, strip, check_only=True)
        if git_ok:
            # Apply via git
            apply_ok, apply_msg = try_git_apply(patch_text, directory, strip, check_only=False)
            if apply_ok:
                return {
                    "status": "success",
                    "operation": "apply",
                    "engine": "git",
                    "files_modified": summary["files"],
                    "additions": summary["additions"],
                    "deletions": summary["deletions"],
                    "message": f"Successfully applied remediation patch to {len(summary['files'])} file(s)."
                }

        # Python fallback application
        py_ok, modified, py_msg = apply_pure_python(patch_text, directory, strip, dry_run=False)
        if py_ok:
            return {
                "status": "success",
                "operation": "apply",
                "engine": "python",
                "files_modified": summary["files"],
                "backup_created": True,
                "additions": summary["additions"],
                "deletions": summary["deletions"],
                "message": f"Successfully applied remediation patch to {len(summary['files'])} file(s). Backups created as .kestrel.bak."
            }
        else:
            return {
                "status": "error",
                "operation": "apply",
                "error": py_msg,
                "message": f"Failed to apply patch: {py_msg}"
            }

    elif op in ("revert", "rollback"):
        # Revert via git
        git_ok, git_msg = try_git_apply(patch_text, directory, strip, check_only=False, reverse=True)
        if git_ok:
            return {
                "status": "success",
                "operation": "revert",
                "engine": "git",
                "files_reverted": summary["files"],
                "message": f"Successfully reverted patch on {len(summary['files'])} file(s)."
            }
            
        # Or restore .kestrel.bak files
        restored = []
        for f in summary["files"]:
            target = os.path.join(directory, f)
            bak = target + ".kestrel.bak"
            if os.path.exists(bak):
                shutil.move(bak, target)
                restored.append(f)
                
        if restored:
            return {
                "status": "success",
                "operation": "revert",
                "engine": "backup_restore",
                "files_reverted": restored,
                "message": f"Successfully restored {len(restored)} file(s) from .kestrel.bak."
            }
        else:
            return {
                "status": "error",
                "operation": "revert",
                "message": "Revert failed: git reverse failed and no .kestrel.bak files found."
            }

    else:
        return {
            "status": "error",
            "message": f"Unsupported operation '{op}'. Supported: check, apply, revert."
        }

def parse_args():
    args = {
        "operation": "check",
        "patch": "",
        "directory": ".",
        "strip": 1
    }
    
    if len(sys.argv) <= 1:
        return args
        
    first_arg = sys.argv[1].strip()
    if first_arg.startswith("{") and first_arg.endswith("}"):
        try:
            data = json.loads(first_arg)
            if isinstance(data, dict):
                args.update(data)
                return args
        except Exception:
            pass

    idx = 1
    positional = []
    while idx < len(sys.argv):
        arg = sys.argv[idx]
        if arg in ("--operation", "-o") and idx + 1 < len(sys.argv):
            args["operation"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--patch", "-p") and idx + 1 < len(sys.argv):
            args["patch"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--directory", "-d", "-C") and idx + 1 < len(sys.argv):
            args["directory"] = sys.argv[idx + 1]
            idx += 2
        elif arg in ("--strip", "-s") and idx + 1 < len(sys.argv):
            try:
                args["strip"] = int(sys.argv[idx + 1])
            except ValueError:
                pass
            idx += 2
        else:
            positional.append(arg)
            idx += 1

    if len(positional) > 0:
        args["operation"] = positional[0]
    if len(positional) > 1:
        args["patch"] = positional[1]
    if len(positional) > 2:
        args["directory"] = positional[2]
    if len(positional) > 3:
        try:
            args["strip"] = int(positional[3])
        except ValueError:
            pass

    return args

def main():
    config = parse_args()
    if not config.get("patch"):
        print(json.dumps({
            "status": "error",
            "message": "Missing required 'patch' parameter (unified diff string or patch file path)."
        }, indent=2))
        sys.exit(1)

    result = execute_patch_op(
        operation=config.get("operation", "check"),
        patch_text=config["patch"],
        directory=config.get("directory", "."),
        strip=int(config.get("strip", 1))
    )
    print(json.dumps(result, indent=2, ensure_ascii=False))
    if result.get("status") in ("error", "conflict"):
        sys.exit(1)

if __name__ == "__main__":
    main()

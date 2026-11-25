# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///
"""Convert an Ossie model to AML with ossie-holistics, and mark each file with its source.

Run it with uv, which brings the Python it needs:

    uv run scripts/convert.py <ossie yaml> -o <aml dir> --sql-dialect <dialect> \
        --data-source-name <name> [--issues <file>] [--repo <dir>] [--force]

--sql-dialect is upper-cased, as the converter takes it. It and the other
options, such as --data-source-name, are passed to `ossie-holistics to-aml`.
The converter runs into a temporary folder. Each `.aml` file it writes then
gains `// ossie-source:` lines, after the converter's Apache license header,
and is written to the output folder:

    // ossie-source: The Ossie model this file was converted from. Keep these lines.
    // ossie-source: path=semantic/sales.ossie.yaml hash=5ba60144 commit=6f1e0c2a9b3d

  path    the Ossie file's path in its git repository, with `/` separators, or
          its name when it isn't in one. A path with a space or a `"` is
          written as a JSON string.
  hash    the first 8 hex digits of the SHA-256 of the Ossie file's bytes.
  commit  the git repository's HEAD commit, when the Ossie file is the same as
          in HEAD. It is left out for a file that is untracked, or changed.

A file the converter writes under `modules/` (an AML module, such as
`modules/reporting/calendar.model.aml`) goes to the `modules/` folder at the
repo's root instead, because anfra reads modules only from there. The repo is
--repo, or else the nearest folder above the output folder that has `.anfra/`.

The models keep a data source name the Ossie file sets itself, in a Holistics
extension, over --data-source-name. The script prints each name the AML uses
that isn't --data-source-name, so it can be added to the repo's data sources.

The converter is the first of: $OSSIE_HOLISTICS (a path to the executable),
`ossie-holistics` on the PATH, or `uv tool run` from the converter's git
repository.

The exit code is the converter's: 1 when its issue log has an ERROR, or when
it failed. Files are written in both cases where the converter wrote them.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

CONVERTER_SOURCE = "git+https://github.com/apache/ossie-converters#subdirectory=holistics"
PREFIX = "// ossie-source:"
NOTE = f"{PREFIX} The Ossie model this file was converted from. Keep these lines."
LICENSE_FIRST_LINE = "// Licensed to the Apache Software Foundation (ASF) under one"
DATA_SOURCE_NAME = re.compile(r"^\s*data_source_name:\s*'([^']*)'", re.MULTILINE)


def fail(message: str) -> None:
    print(f"Error: {message}", file=sys.stderr)
    sys.exit(2)


def converter() -> list[str]:
    """The command that runs ossie-holistics."""
    if os.environ.get("OSSIE_HOLISTICS"):
        return [os.environ["OSSIE_HOLISTICS"]]
    if shutil.which("ossie-holistics"):
        return ["ossie-holistics"]
    # `uv run` sets $UV to its own path, which works even when uv isn't on the PATH.
    exe = "uv.exe" if os.name == "nt" else "uv"
    for uv in (os.environ.get("UV"), shutil.which("uv"), str(Path.home() / ".local" / "bin" / exe)):
        if uv and Path(uv).is_file():
            return [uv, "tool", "run", "--from", CONVERTER_SOURCE, "ossie-holistics"]
    fail("can't find ossie-holistics, or uv to run it. Install uv, then run this script with `uv run`.")
    return []


def git(directory: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(directory), *args], capture_output=True, text=True, check=False
    )


def source_line(ossie: Path) -> str:
    """The `// ossie-source: path=… hash=… [commit=…]` line for the Ossie file."""
    digest = hashlib.sha256(ossie.read_bytes()).hexdigest()[:8]
    path, commit = ossie.name, None
    try:
        top = git(ossie.parent, "rev-parse", "--show-toplevel")
        if top.returncode == 0:
            path = ossie.resolve().relative_to(Path(top.stdout.strip()).resolve()).as_posix()
            tracked = git(ossie.parent, "ls-files", "--error-unmatch", "--", ossie.name).returncode == 0
            unchanged = git(ossie.parent, "diff", "--quiet", "HEAD", "--", ossie.name).returncode == 0
            head = git(ossie.parent, "rev-parse", "--verify", "--quiet", "--short=12", "HEAD")
            if tracked and unchanged and head.returncode == 0:
                commit = head.stdout.strip()
    except (OSError, ValueError):
        pass  # git isn't installed, or the path can't be made relative: keep the file's name.
    value = json.dumps(path) if any(c.isspace() or c == '"' for c in path) else path
    return f"{PREFIX} path={value} hash={digest}" + (f" commit={commit}" if commit else "")


def with_source(text: str, header: str) -> str:
    """The text with `header` after the converter's leading Apache license comment, or first if it has none."""
    lines = text.split("\n")
    if not lines or lines[0] != LICENSE_FIRST_LINE:
        return header + text
    end = 0
    while end < len(lines) and lines[end].startswith("//"):
        end += 1
    if end < len(lines) and lines[end] == "":
        end += 1
    return "\n".join(lines[:end]) + "\n" + header + "\n".join(lines[end:])


def repo_root(output: Path) -> Path:
    """The nearest folder at or above `output` that has `.anfra/`, or the current folder."""
    for folder in [output.resolve(), *output.resolve().parents]:
        if (folder / ".anfra").is_dir():
            return folder
    return Path.cwd()


def target_of(relative: Path, output: Path, repo: Path) -> Path:
    """Where a file the converter wrote at `relative` goes: modules to the repo's `modules/`."""
    if relative.parts[0] == "modules":
        return repo / relative
    return output / relative


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Convert an Ossie model to AML, and mark each file with the Ossie file it came from.",
        epilog="--sql-dialect, --data-source-name and any other options are passed to "
        "`ossie-holistics to-aml`. See `ossie-holistics to-aml --help`.",
    )
    parser.add_argument("input", metavar="OSSIE_YAML", help="the Ossie model to convert")
    parser.add_argument("-o", "--output", required=True, metavar="DIR", help="the folder to write the AML files to")
    parser.add_argument(
        "--sql-dialect", required=True, type=str.upper, metavar="DIALECT",
        help="the SQL dialect of the data source's warehouse, such as BIGQUERY or SNOWFLAKE, in any case",
    )
    parser.add_argument("--data-source-name", metavar="NAME", help="the data source the models read from")
    parser.add_argument("--issues", metavar="FILE", help="write the issue log here, instead of to stderr")
    parser.add_argument(
        "--repo", metavar="DIR",
        help="the anfra repo, whose modules/ folder gets the AML modules "
        "(default: the nearest folder above the output folder that has .anfra/)",
    )
    parser.add_argument("--force", action="store_true", help="overwrite existing files")
    args, passed = parser.parse_known_args()
    passed = ["--sql-dialect", args.sql_dialect, *passed]
    if args.data_source_name:
        passed += ["--data-source-name", args.data_source_name]

    ossie = Path(args.input)
    output = Path(args.output)
    if not ossie.is_file():
        fail(f"{ossie} isn't a file.")
    if output.exists() and not output.is_dir():
        fail(f"{output} exists and isn't a folder.")

    header = f"{NOTE}\n{source_line(ossie)}\n\n"
    command = converter()

    with tempfile.TemporaryDirectory(prefix="ossie-aml-") as tmp:
        # The converter writes into a temporary folder, the issue log included, so that
        # nothing reaches the output until every file is checked.
        staged = Path(tmp) / "aml"
        staged_issues = Path(tmp) / "issues.yaml"
        issues = ["--issues", str(staged_issues)] if args.issues else []
        code = subprocess.run([*command, "to-aml", str(ossie), "-o", str(staged), *issues, *passed]).returncode
        written = sorted(p for p in staged.rglob("*") if p.is_file()) if staged.is_dir() else []
        if not written:
            return code or 1

        repo = Path(args.repo) if args.repo else repo_root(output)
        targets = [(p, target_of(p.relative_to(staged), output, repo)) for p in written]
        if args.issues:
            targets.append((staged_issues, Path(args.issues)))
        existing = [str(t) for _, t in targets if t.exists()]
        if existing and not args.force:
            fail(f"these files already exist, so nothing was written. Pass --force to overwrite them: {', '.join(existing)}")

        names: set[str] = set()
        for source, target in targets:
            target.parent.mkdir(parents=True, exist_ok=True)
            if source.suffix == ".aml":
                text = source.read_text(encoding="utf-8")
                names.update(DATA_SOURCE_NAME.findall(text))
                target.write_text(with_source(text, header), encoding="utf-8", newline="\n")
            else:
                shutil.copyfile(source, target)
        print(f"Wrote {len(written)} file(s) to {output}.", file=sys.stderr)
        modules = sorted({r.parts[1] for r in (p.relative_to(staged) for p in written) if r.parts[0] == "modules" and len(r.parts) > 2})
        if modules:
            print(
                f"Wrote the AML module(s) {', '.join(modules)} to {repo / 'modules'}, "
                "because anfra reads modules only from there.",
                file=sys.stderr,
            )
        other = sorted(names - {args.data_source_name})
        if other:
            print(
                f"The AML reads from data sources other than --data-source-name: {', '.join(other)}. "
                "The Ossie file sets these itself. Each must be in .anfra/data_sources.yml, "
                "or be renamed in the AML.",
                file=sys.stderr,
            )
        if code == 1:
            where = args.issues or "the issue log above"
            print(f"The conversion has at least one ERROR issue. Read {where}.", file=sys.stderr)
    return code


if __name__ == "__main__":
    sys.exit(main())

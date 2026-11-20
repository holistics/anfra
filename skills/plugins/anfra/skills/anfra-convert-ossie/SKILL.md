---
name: anfra-convert-ossie
description: Convert an Apache Ossie (Open Semantic Interchange, OSI) semantic model, a `*.ossie.yaml` file, into anfra AML models and a dataset with the `ossie-holistics` converter, installing what it needs (uv, which brings its own Python) on Windows, macOS or Linux. Use when the user has an Ossie or OSI file, or a semantic model another tool exported to Ossie, and wants it in anfra.
---

# Converting an Ossie model to AML

`ossie-holistics` is a command-line converter that turns one Ossie file into one `.model.aml` per
Ossie dataset and one `.dataset.aml` for the whole model. It runs on the user's machine, and only
converts Ossie to AML, not back.

Done means: the generated AML passes `anfra validate`.

The user may never have installed Python or used a terminal. Do the steps for them, ask before you
install anything, and explain each install in one sentence.

## 1. Check the repo

The converter writes AML that reads from a data source in `.anfra/data_sources.yml`. If the
folder has no `.anfra/`, or that file has no data source, set the repo up with
[](../anfra-setup-repo/) first, then come back.

## 2. Get the converter running

The converter is a Python tool. uv runs it, and downloads the Python it needs, so the user doesn't
install Python.

**uv.** Run `uv --version`. If it fails, ask the user before installing uv. On macOS and Linux:

```sh
curl -LsSf https://astral.sh/uv/install.sh | sh
```

On Windows, from any shell:

```sh
powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"
```

Your shell won't find `uv` until it restarts, so call it by the path the installer prints:
`~/.local/bin/uv` on macOS and Linux, `%USERPROFILE%\.local\bin\uv.exe` on Windows.

**git.** uv downloads the converter from its git repository, which needs git. Run `git --version`.
If it fails, ask the user before installing git:

| OS | Install git |
|---|---|
| macOS | `xcode-select --install`. It opens a dialog, so tell the user to accept it, and wait until they say it's done. |
| Windows | `winget install --id Git.Git -e --source winget` |
| Linux | The distribution's package manager, such as `sudo apt install git`. It asks for the user's password, so ask them to run it. |

**The converter.** Don't run `ossie-holistics` yourself. Run this skill's
`scripts/convert.py` with uv, which runs the converter for you. It uses `ossie-holistics` if the
user has installed it, and otherwise has uv download it on first use. Check that it runs:

```sh
uv run <this skill's folder>/scripts/convert.py --help
```
## 3. Convert

Run the command from the repo's folder. Write it on one line, because a `\` line break fails in
PowerShell and cmd:

```sh
uv run <this skill's folder>/scripts/convert.py <file> -o models/<name>/ --sql-dialect <dialect> --data-source-name <data source> --issues models/<name>/ossie-issues.yaml
```

- `--sql-dialect` is the SQL dialect of the data source's warehouse, such as `BIGQUERY` or
  `SNOWFLAKE`. If you pass one the converter doesn't take, it lists the ones it does. If the Ossie
  file's expressions name no dialect and you can't tell which warehouse the data source is, ask the
  user.
- `--data-source-name` is the data source's name in `.anfra/data_sources.yml`.
- `--issues` writes the issue log to a file, so you can read it whole.

Exit code `1` means the log has at least one ERROR. That is a result to deal with in step 5, not a
failed install.

For several Ossie files, convert each one into its own folder under `models/`.

## 4. Check the data sources

An Ossie file exported from Holistics keeps its own data source names, and the models use them
instead of `--data-source-name`. The script then prints the names the AML reads from that aren't
`--data-source-name`. For each one that isn't in `.anfra/data_sources.yml`, ask the user whether
to add it as a data source, or to change it in the AML to the data source they named.

## 5. Work through the issues

The log has three levels:

- **ERROR**: the converter couldn't convert it, such as a metric with a window function. Try to
  write it by hand, as an AQL metric on the dataset, with [](../anfra-aml/) and
  [](../anfra-aql/). If AQL can't express it, leave it out and tell the user.
- **WARNING**: the converter dropped or changed it, such as another tool's vendor extension. Tell
  the user about each one that changes what a field or metric returns.
- **INFO**: the converter kept it in another form. Give the user a count, not the list.

Never invent an AML property for something the converter dropped.

## 6. Check the result

```sh
anfra validate
```

Fix what it reports. A model's or dataset's name must be unique across the repo, so a
`Duplicated name` diagnostic means another file already uses it, often another Ossie file over the
same tables. If the two models differ, ask the user which one to rename. Then rename it in its
`.model.aml`, and everywhere the dataset names it.

Then tell the user:

1. which files were written,
2. what couldn't be converted, from the ERRORs you couldn't fix,
3. what you changed by hand, such as a metric rewritten in AQL or a model renamed.

## Related skills

- [](../anfra-aml/): change the generated models and dataset.
- [](../anfra-docs/): look up anfra's public docs.

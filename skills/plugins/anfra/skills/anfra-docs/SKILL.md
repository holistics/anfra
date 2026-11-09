---
name: anfra-docs
description: Look up the anfra and AMQL docs — concepts, guides and reference for AML, AQL, modeling patterns, joins, the anfra CLI, the SDK, Data Apps and the semantic catalog. Use when the other anfra skills don't cover what you need, or to check how something in anfra or AMQL works rather than guessing.
---

# anfra and AMQL docs

Two sites hold the docs, and both can be read as markdown:

| Site | Covers | Index |
|---|---|---|
| https://amql.org/docs | AML and AQL: concepts, modeling patterns, joins, reusability, functions | https://amql.org/llms.txt |
| https://anfra.ai/docs | anfra: setup, data sources, the CLI, the SDK and Data Apps, the semantic catalog | https://anfra.ai/docs/llms.txt |

1. Fetch the index of the site that covers your question. It lists every page with a one-line
   summary.
2. Fetch the pages you need with `.md` added to the path, such as
   https://amql.org/docs/aml/concepts.md, to get them as markdown.

Don't fetch `llms-full.txt`, which is the whole site in one file and too big to read.

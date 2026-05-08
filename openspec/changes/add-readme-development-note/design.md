# Design: Development Notes Section

## Approach

Append the following section to the end of `README.md`:

```markdown
## Development Notes

This repository is a Scion test project.

### Inspecting the project locally

Clone the repository:

```bash
git clone https://github.com/dodwyer/scion-test.git
cd scion-test
```

Browse the file tree to explore the project structure:

```bash
ls -la
```

### Running

There is no build system or runtime in this project. To inspect or experiment locally, open the files directly in your editor of choice.
```

## File Affected

| File | Change |
|------|--------|
| `README.md` | Append `## Development Notes` section |

## Implementation Steps

1. Open `README.md` (currently contains only `# scion-test`).
2. Append the `## Development Notes` section as specified above.
3. Verify the markdown renders correctly.

## Risks

None — this is documentation-only and cannot break any runtime behavior.

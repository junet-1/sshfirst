# Workspace format

SSH First workspaces are declarative, UTF-8 JSON documents. They describe
resources and a recursive layout; they never contain widget, DOM, WebKit, or
terminal emulator state.

The current format version is `1`:

```json
{
  "version": 1,
  "name": "HomeLab",
  "resources": {
    "homeserver": {
      "type": "ssh",
      "host": "homeserver"
    },
    "grafana": {
      "type": "browser",
      "url": "https://grafana.example.com"
    }
  },
  "layout": {
    "type": "split",
    "direction": "horizontal",
    "children": [
      {
        "type": "terminal",
        "resource": "homeserver",
        "ratio": 0.35
      },
      {
        "type": "browser",
        "resource": "grafana",
        "ratio": 0.65
      }
    ]
  }
}
```

## Nodes

- `split` has a `direction` (`horizontal` or `vertical`) and one or more
  recursive `children`.
- `terminal`, `browser`, and `sftp` are leaves and refer to a resource by ID.
- A positive `ratio` may be placed on any child of a split. Missing ratios use
  weight `1`; all sibling ratios are normalized when the file is parsed.

Unknown node types are retained when a document is read and written. During
restore they are skipped unless a plugin has registered a factory for that
type. This lets future nodes such as RDP, VNC, Docker, Kubernetes, and Notes be
added without changing the recursive parser.

## Resources

Resources are separate from the layout so several leaves can reference the
same target. SSH First writes both the human-readable host label and its local
`hostId` when the target comes from the host catalog. Resolution prefers the
stable ID and reads the host's current settings, so editing a host updates every
workspace that references it. Imported files can omit `hostId` and resolve by
label or hostname instead.

Browser resources contain a URL. Browser tabs backed by a Web host also carry
the host reference, so later URL changes are picked up automatically.

## Complete tab restoration

The minimal portable format needs only `resources` and `layout`. Files saved by
SSH First additionally contain:

- `tabs`: the ordered declarations for all open tabs, including tabs not
  currently visible in a split.
- `activeTab`: the ID of the selected tab.

These are optional version-1 fields, so the minimal format above and files from
other tools remain valid.

Additional top-level, node, and resource properties are preserved. Features
such as auto reconnect, templates, startup or pinned workspaces, encryption,
variables, and plugin metadata can therefore be introduced additively. A
format version increase is reserved for incompatible semantic changes.

## Storage and transport

Managed files live under `$XDG_DATA_HOME/ssh-first/workspaces` (normally
`~/.local/share/ssh-first/workspaces`). Writes are atomic and files use mode
`0600`. The workspace dialog can import or export standalone `.json` files.

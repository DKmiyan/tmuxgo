# tmuxgo stdio bridge v0

This optional machine interface keeps the existing TUI unchanged. It runs for
one stdio connection, reads only tmux metadata, and never captures pane content,
sends input, resumes an agent, edits tmux.conf, or installs a background daemon.
Native display/input uses a separate ordinary TTY attach process.

## Processes

```text
tmuxgo bridge --socket default
tmuxgo bridge --socket work --interval-ms 1000
tmuxgo bridge --protocol-version
tmuxgo bridge-attach --ticket <one-use-ticket>
```

`--socket` is an explicit tmux `-L` name, `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`;
`default` names the default server. Arbitrary `-S` paths are not accepted.
The bridge ignores inherited TMUX/TMUX_PANE context. The attach process uses
the socket embedded in its ticket. Both processes must run as the same remote
user with the same temporary-directory environment (as on one SSH connection).

The bridge writes only UTF-8 NDJSON to stdout. Human errors go to stderr through
the existing English/Chinese i18n table. The TTY attach writes native terminal
bytes and is never fed to the NDJSON parser. `--protocol-version` prints
`{"version":0}` without connecting to a tmux server.

## Envelopes

Every request has exactly four fields:

```json
{"version":0,"requestId":"r1","command":"snapshot","params":{}}
```

A successful request returns:

```json
{"version":0,"type":"response","requestId":"r1","ok":true,"result":{}}
```

A rejected request returns `ok:false` and `error:{code,message}`, without result.
The message is localized and contains no native stderr, arguments, paths or
transcript content; clients should branch on code. Invalid framing without a
usable requestId, EOF, or a transport write error terminates the bridge and
cleans its private ticket directory. Ordinary command rejection does not close
an otherwise healthy stream.

At startup and every interval, the bridge also writes
`{version:0,type:"snapshot",snapshot:Snapshot}`. A `snapshot` request returns
exactly the same Snapshot shape in result. A trusted sample cannot be produced
when the server is inaccessible or malformed: the bridge exits instead of
publishing a false zero tree. A verified absent server is a valid empty tree.

## Snapshot (TypeScript-friendly example)

```ts
const snapshot = {
  bridgeId: "0123456789abcdef0123456789abcdef",
  sequence: 1,
  observedAt: "2026-09-07T00:00:00Z",
  server: {
    socketName: "default",
    generation: "server:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    running: true,
    tmuxVersion: "tmux 3.6a",
  },
  sessions: [{
    id: "$0", name: "research", attached: true,
    windows: [{
      id: "@0", sessionId: "$0", index: 0, name: "editor", active: true,
      layout: "b25d,80x24,0,0,0",
      panes: [{
        id: "%0", windowId: "@0", index: 0, active: true,
        cwd: "/work/research", currentCommand: "bash",
      }],
    }],
  }],
  attachments: [{
    attachmentId: "attach:0123456789abcdef0123456789abcdef",
    state: "attached", sessionId: "$0", windowId: "@0", paneId: "%0",
  }],
};
```

Session/window/pane IDs are native `$digits`, `@digits`, `%digits` (1–10 digits).
Names are at most 512 UTF-8 bytes, cwd 4096, currentCommand 256 and layout 65536;
control characters are rejected. No argv, pane text, PID, socket pathname,
client TTY or ticket secret appears in a snapshot. Arrays are always arrays,
including for an absent server. `generation` is always an opaque `server:` plus
64 lower-case hex digits, including the verified-absence state. It derives from
socket selection and, when running, server PID/start_time and socket dev/inode.
`bridgeId` is 32 random lower-case hex digits and changes on every bridge run.
Sequence increases for every observation; gaps are normal.

Attachment state is `pending | attached | detached | expired`. Its three native
IDs are either all available for the exact connected client or null. An
attachment reports where its real TTY client currently is, including switches
made through native tmux/tmuxgo. Session-level active windows remain tmux's
native shared state; the bridge does not split a Window into local PTYs.

## Commands and parameters

All mutation parameters include **expectedGeneration** and **operationId**.
Both are required even for an initial session.create against an absent server.
The generation comes from the latest observed snapshot. operationId and
requestId use `[A-Za-z0-9_.:-]{1,128}`. Names/cwd are JSON data, never raw commands.
Create cwd must be an existing absolute directory; this API does not mkdir.
Create name may be empty (tmux chooses it); rename name is nonempty, max 128 bytes.

| Command | Additional required params | Optional params |
| --- | --- | --- |
| snapshot | none (no mutation fields) | none |
| attach.status | attachmentId (no mutation fields) | none |
| session.create | name, cwd | none |
| session.select | sessionId, attachmentId | none |
| session.rename | sessionId, name | none |
| session.kill | sessionId, confirm:true | none |
| window.create | sessionId, name, cwd | none |
| window.select | sessionId, windowId, attachmentId | none |
| window.rename | sessionId, windowId, name | none |
| window.kill | sessionId, windowId, confirm:true | none |
| pane.select | sessionId, windowId, paneId, attachmentId | none |
| pane.split | sessionId, windowId, paneId, direction, cwd | none |
| attach.issue | sessionId | windowId |

`direction` is `horizontal` (side by side) or `vertical` (stacked). Selection
uses the exact client associated with attachmentId; it never guesses a client
from a session name. Parent membership is checked against native IDs. Native
composite targets keep window/pane operations inside their specified session.
Kills require confirm:true and have tmux's normal cascading behavior.
Success is returned only after a fresh metadata sample confirms that the target
native session/window is gone. `window.kill` removes all links to that same
native window if it is linked into several sessions. Other native windows are
not killed. This ends tmux's corresponding PTYs; independently daemonized or
detached descendant processes are outside tmux's process-lifetime guarantee.

Mutation results are `{generation, sessionId?, windowId?, paneId?, attachmentId?}`.
Created native IDs are returned by tmux's receipt. Periodic snapshots publish
the resulting tree. attach.issue additionally returns `ticket` and `expiresAt`.
attach.status returns the Attachment shape above. No mutation response contains
a transcript or arbitrary native command output.

## Generation and argv safety

All tmux access goes through the internal/tmux backend, using exec argv arrays.
Existing-server mutations and native attach are guarded at the tmux execution
point by `if-shell -F`: it evaluates a tmux format, **not a POSIX shell**. Its
native command-list branch is compiled exclusively from allowlisted operations,
validated native targets, and individually encoded argument values. The API
never accepts an arbitrary command list. This is needed because a pre/post
CLI check alone could act on reused IDs after a server restart.

The backend verifies socket identity and server generation before operations,
and results are checked afterward. Creating the first session is the explicit
case that starts a server; killing its final session may return a new verified
absence generation. A changed running server returns SERVER_CHANGED. Native
names containing quote characters, backslashes, semicolons, dollar syntax,
backticks, format-looking text and Unicode remain data.

## Attach ticket lifecycle

The returned ticket has the form
`v0.<32hex bridgeId>.<32hex ticketId>.<64hex secret>` and expires after 30 seconds.
It is a capability: only forward it to the intended `bridge-attach` process;
do not log it or include it in audit/status output. Files are 0600 beneath an
owned 0700 temporary directory. Claim uses an atomic one-use rename and checks
the secret, expiry, bridge lifetime and expected server generation.

The attach helper records its real TTY and the native tmux attach client's PID
privately. The bridge correlates both with list-clients; these values never
leave metadata stdout. The helper watches its bridge lease and process lifetime.
On lost stdio/SSH/bridge it terminates only its own tmux **client**, leaving all
remote sessions and tasks alive. Exit removes its claim; bridge exit removes
only its own ticket directory/cache. There is no kill-server cleanup.

## Bounds and compatibility

- Request JSON: 64 KiB per line; duplicate/unknown fields, invalid UTF-8, extra
  JSON and unknown commands are refused. Snapshot output: at most 2 MiB.
- At most 256 sessions, 1024 windows and 4096 panes in one metadata tree.
- At most 64 retained ticket/attachment records; pending tickets expire in 30s.
- At most 256 operation results for 5 minutes. Identical retained operationId
  plus params returns the prior result; different params returns
  OPERATION_CONFLICT. Deduplication is not permanent across eviction or restart.
- The snapshot interval is 250–10000ms, default 1000ms. Individual native
  commands have a 5s deadline and bounded output.
- tmux format fields use TAB (compatible with the tmux 3.4+ baseline). The
  implementation is exercised with a real isolated server; no user server,
  authentication, model, pane transcript or tmux.conf is used by those tests.

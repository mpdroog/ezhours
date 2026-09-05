# EZHours

A simple time tracking app that lives in your system tray.

## Features

- Start/stop timer from system tray
- Tracks which apps you use during work sessions
- Saves entries with project, description, and app usage data
- Syncs the `hours/` folder between devices over git

## Install

```bash
go build -o ezhours
```

## Usage

Run the app and click the tray icon to start/stop tracking. When you stop, a dialog lets you save the entry with a project name and description.

## Syncing between devices

The `hours/` folder can be a git repository of its own, which EZHours keeps in
sync so the same time sheets are usable from several machines. It uses
[go-git](https://github.com/go-git/go-git), so no `git` binary is required.

It pulls when the app starts and when you press *Start Timer*, and commits and
pushes right after you save an entry. There is also a *Sync Now* menu item,
which shows the time of the last successful sync or the reason the last one
failed.

A failed sync also badges the tray icon with an amber triangle, so an
unpublished folder is visible without opening the menu; it clears on the next
successful sync. Nothing is lost either way -- entries are committed locally
before the push, so a failed sync only delays publishing.

### Setup

1. Create an empty (private) repository on GitHub.

2. Generate a dedicated deploy key and give it **write** access under the
   repository's *Settings → Deploy keys*:

   ```bash
   ssh-keygen -t ed25519 -f ~/.ezhours.priv -C ezhours
   cat ~/.ezhours.priv.pub
   ```

   EZHours always uses `~/.ezhours.priv`; it never touches your ssh-agent or
   your other keys. If the key has a passphrase, put it in
   `EZHOURS_KEY_PASSPHRASE`.

3. Point the hours folder at the repository, either once by hand:

   ```bash
   git -C hours init
   git -C hours remote add origin git@github.com:you/hours.git
   ```

   or by setting `EZHOURS_REMOTE`, which EZHours applies on startup when the
   folder has no `origin` yet:

   ```bash
   EZHOURS_REMOTE=git@github.com:you/hours.git ./ezhours
   ```

4. Repeat steps 2 and 3 on your other devices. The first sync merges whatever
   each device already had.

Without a remote configured EZHours still commits every entry locally, so you
keep a history either way.

### Merging

When two devices both logged time since the last sync, git cannot fast-forward.
EZHours then merges the `.txt` files itself: it parses them into date blocks and
time entries and takes the union of both sides, against the merge base. Entries
added on either device are kept, entries you deleted on one device stay deleted,
and an entry edited on one side wins over the untouched side. If the same entry
was edited on both devices, both descriptions are kept so nothing is lost.

Files that are not `.txt` are not merged; the local version wins.

## Autostart on Linux

`contrib/ezhours.service` runs EZHours as a `systemd --user` service, started at
login:

```bash
go build -o ezhours
ln -s $PWD/contrib/ezhours.service ~/.config/systemd/user/ezhours.service
systemctl --user daemon-reload
systemctl --user enable --now ezhours
```

The unit pins `WorkingDirectory` to the repository, because the hours folder is a
relative path. `EZHOURS_REMOTE` and `EZHOURS_KEY_PASSPHRASE` are read from
`~/.config/ezhours.env`. The comments in the unit cover the rest.

## Platforms

- macOS
- Windows
- Linux (tested on Mint)

The tray icon is a macOS template icon: a black glyph that macOS inverts for a
dark menu bar. Linux tray hosts draw it as handed to them, so on Linux EZHours
picks the glyph colour itself, from the colour scheme of the GTK theme. If it
guesses wrong -- a dark theme whose name does not say "dark", say -- set
`EZHOURS_ICON_COLOR` to `light` or `dark` and restart.

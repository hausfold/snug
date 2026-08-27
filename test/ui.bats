#!/usr/bin/env bats
# Unit tests for share/ui.sh — snug's bash half, run by the `bash` CI job.
#
# The width sweeps are the point and are what to run first when anything in
# there changes. Everything this library exists for is one rule —
#
#   nothing it draws may reach the terminal's last column
#
# — and every defect it was written for is a violation of it: a `%-34s` row that
# is 48 cells whatever the job is called, a `\r` into the middle of a wrapped
# line, a `printf` that padded by bytes. "Should rarely" is not the promise;
# `may not, at any width, in any tier` is.
#
# ── two things the harness does on purpose ───────────────────────────────────
# SNUG_ASCII=1 in the sweeps, so that one byte is one character is one cell and
# `${#s}` is a cell count in ANY locale — CI's included, where `LANG` is unset
# and a `✓` would otherwise measure three. The arithmetic under test does not
# know which alphabet it is in; the alphabet gets its own test below.
#
# UI_TTY / UI_COLS / UI_AVAIL are set by hand rather than by a pty, the same way
# the Go tests construct a `Term`. That is what makes a 199-width sweep cost one
# fork instead of two hundred. The one thing a pty is genuinely needed for —
# `stty size` against a real window — gets a real pty at the bottom.

setup() {
  UI="$BATS_TEST_DIRNAME/../share/ui.sh"
  # "$BASH", not a bare `bash`: this needs associative arrays and `${v,,}`, and
  # macOS still ships 3.2 as /bin/bash. A bare name would fail on the array
  # rather than on the thing under test.
  unset NO_COLOR CLICOLOR_FORCE SNUG_VARIANT SNUG_ASCII COLUMNS
}

# ui_sh <snippet> — run <snippet> with the library sourced, in its own shell.
#
# SNUG_ASCII=1 is set HERE rather than exported, so the tests that are about the
# alphabet can build their own environment without it. Everything else wants the
# ASCII marks: one byte, one character, one cell, in CI's C locale and in a
# developer's UTF-8 one alike, so `${#s}` is a cell count either way and the
# expected strings do not depend on which machine ran them.
ui_sh() {
  run env SNUG_ASCII=1 "$BASH" -c "set -euo pipefail; source '$UI'; $1"
}

# The preamble every sweep shares: a strip-the-escapes helper (pure bash, so the
# sweep stays one process) and three rows with the shapes that have historically
# broken things — a running spinner, a short name, a name far past any budget,
# and the eight-cell duration a hardcoded seven overflowed on.
SWEEP_PRELUDE='
bare() { # bare <string> — the string with every SGR escape removed
  local s="$1" out=""
  while [[ "$s" =~ ^([^$'"'"'\033'"'"']*)$'"'"'\033'"'"'\[[0-9\;]*m(.*)$ ]]; do
    out+="${BASH_REMATCH[1]}"; s="${BASH_REMATCH[2]}"
  done
  printf "%s" "$out$s"
}
rows() {
  ui_clear
  ui_row run  "build"                                   "100m 05s"
  ui_row ok   "lint"                                    "3s"
  ui_row fail "a-job-name-far-longer-than-any-window"   "12m 34s"
  ui_row wait "test"                                    "queued"
}
'

# ── the one rule ─────────────────────────────────────────────────────────────

@test "a live region never reaches the last column, at any width, painted or plain" {
  # The sweep runs TWICE: colourless, and with colour forced on. That second
  # pass is not padding. snug's own sweep runs colourless only, and it is why
  # its `bare` tier shipped two cells over the edge — `TrimRight(mark, " ")`
  # silently does nothing once the mark ends in a reset escape, so the gutter
  # it thought had collapsed to one space was still carrying three.
  ui_sh "$SWEEP_PRELUDE"'
    UI_TTY=1
    for colour in truecolor 256 16 none; do
      UI_PROFILE=$colour; ui__resolve_palette
      for w in $(seq 2 200); do
        UI_COLS=$w; UI_AVAIL=$(( w - 1 ))
        rows
        lines=(); ui__layout lines
        for l in "${lines[@]}"; do
          b="$(bare "$l")"
          if [ "${#b}" -gt "$UI_AVAIL" ]; then
            echo "width $w, $colour: ${#b} cells over a limit of $UI_AVAIL: [$b]"
            exit 1
          fi
        done
      done
    done
    echo SWEPT'
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  [[ "$output" == *SWEPT* ]]
}

@test "prose folds to the cap and never reaches the last column" {
  ui_sh "$SWEEP_PRELUDE"'
    UI_TTY=1
    long="a sentence long enough to fold several times over followed by /a/store/path/far/too/long/to/break/at/a/word/boundary/at/all and more words after it"
    for w in $(seq 2 200); do
      UI_COLS=$w
      UI_PROSE=$w; [ "$UI_PROSE" -gt "$UI_CAP" ] && UI_PROSE=$UI_CAP
      limit=$UI_PROSE
      out="$(ui_say "$long" 2>&1)"
      while IFS= read -r l; do
        b="$(bare "$l")"
        if [ "${#b}" -gt "$limit" ]; then
          echo "width $w: ${#b} cells over a limit of $limit: [$b]"; exit 1
        fi
      done <<<"$out"
    done
    echo SWEPT'
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  [[ "$output" == *SWEPT* ]]
}

@test "the floor is one glyph, and below it nothing is drawn past the edge" {
  # At two columns there is one honest cell. The glyph is what survives — the
  # name, the detail and the gutter are all gone — and it must not be padded
  # back up to three by a helper that always pads.
  ui_sh "$SWEEP_PRELUDE"'
    UI_TTY=1; UI_COLS=2; UI_AVAIL=1
    UI_PROFILE=truecolor; ui__resolve_palette
    ui_clear; ui_row ok build 3s
    lines=(); ui__layout lines
    b="$(bare "${lines[0]}")"
    echo "[$b] ${#b}"'
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  [[ "$output" == "[+] 1" ]] || { echo "got: $output"; false; }
}

# ── the tiers ────────────────────────────────────────────────────────────────

@test "short names keep their detail on a wide terminal" {
  # The inversion `bench` shipped for an afternoon: clamp the column to the
  # content first, then test the CLAMPED value, and you have asked "is the
  # longest name short?" when you meant "is there room?". A CI run of build /
  # test / lint dropped its durations on a 200-column terminal.
  ui_sh '
    UI_TTY=1; UI_COLS=200; UI_AVAIL=199; UI_PROFILE=none; ui__resolve_palette
    ui_clear; ui_row ok build 12s; ui_row ok test 3s; ui_row ok lint 1s
    lines=(); ui__layout lines
    printf "%s\n" "${lines[@]}"'
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  [[ "$output" == *"12s"* ]] || { echo "the detail column vanished on a 200-column window: $output"; false; }
}

@test "a measured detail column fits the eight-cell duration a hardcoded seven overflowed" {
  # `bench` budgeted seven cells because "12m 34s" is the longest duration it
  # imagined. GitHub allows six-hour jobs, and "100m 05s" is eight.
  ui_sh '
    UI_TTY=1; UI_COLS=30; UI_AVAIL=29; UI_PROFILE=none; ui__resolve_palette
    ui_clear; ui_row ok build "100m 05s"
    lines=(); ui__layout lines
    printf "%s|%s\n" "${lines[0]}" "${#lines[0]}"'
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  [[ "$output" == *"100m 05s"* ]] || { echo "got: $output"; false; }
}

@test "the tiers shed the detail before the name, and the padding before the detail" {
  ui_sh '
    UI_TTY=1; UI_PROFILE=none; ui__resolve_palette
    for w in 40 18 10; do
      UI_COLS=$w; UI_AVAIL=$(( w - 1 ))
      ui_clear; ui_row ok build 12s
      lines=(); ui__layout lines
      printf "%s=[%s]\n" "$w" "${lines[0]}"
    done'
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  # 40: table — name padded, detail beside it.
  [[ "$output" == *'40=[   +  build 12s]'* ]] || { echo "got: $output"; false; }
  # 18: list — the detail is the first thing to go.
  [[ "$output" == *'18=[   +  build]'* ]] || { echo "got: $output"; false; }
  # 10: bare — the indent collapses to a single space.
  [[ "$output" == *'10=[+ build]'* ]] || { echo "got: $output"; false; }
}

# ── streams, colour and the alphabet ─────────────────────────────────────────

@test "stdout carries data only; every human line goes to stderr" {
  # The family contract, and the reason `snug run` works as a bash coproc:
  # callers do `cd "$(scruff child …)"` and hooks read paths off fd 1.
  ui_sh '{ ui_say hello; ui_ok fine; ui_warn hm; ui_fail no; } 2>/dev/null; ui_data /the/path'
  [ "$status" -eq 0 ]
  [ "$output" = "/the/path" ] || { echo "stdout carried more than data: [$output]"; false; }
}

@test "a stream with no window is never folded" {
  # The bug scruff found: an acceptance suite greps stderr for whole messages, and
  # every assertion long enough to cross column 80 broke the day it moved onto
  # snug. A pipe has no geometry, and inventing one for it is the `tput cols`
  # mistake wearing different clothes.
  long="still live at /var/folders/9k/xxxxxxxxxxxxxxxxxxxx/T/scruff-test.AbCdEf/repo — nothing was rebuilt"
  ui_sh "ui_say '$long' 2>&1"
  [ "$status" -eq 0 ]
  [ "${#lines[@]}" -eq 1 ] || { echo "a redirected stream was folded: $output"; false; }
  [[ "$output" == *"$long"* ]]
}

@test "no escapes reach a pipe, and the colour precedence is the documented one" {
  ui_sh 'ui_say hello 2>&1 | cat -v'
  [[ "$output" != *"^["* ]] || { echo "an escape reached a pipe: $output"; false; }

  # NO_COLOR beats a terminal…
  run env -u COLORTERM NO_COLOR=1 TERM=xterm-256color "$BASH" -c \
    "set -euo pipefail; source '$UI'; UI_TTY=1; ui__detect_profile; echo \$UI_PROFILE"
  [ "$output" = none ] || { echo "NO_COLOR was ignored: $output"; false; }

  # …and CLICOLOR_FORCE beats NO_COLOR, which is snug's precedence and therefore
  # this one. The two specs disagree (no-color.org against bixense) and the
  # family had to pick; what matters is that both painters picked the same.
  run env -u COLORTERM NO_COLOR=1 CLICOLOR_FORCE=1 TERM=xterm-256color "$BASH" -c \
    "set -euo pipefail; source '$UI'; echo \$UI_PROFILE"
  [ "$output" = 256 ] || { echo "CLICOLOR_FORCE lost to NO_COLOR: $output"; false; }

  run env -u COLORTERM CLICOLOR_FORCE=1 TERM=xterm-256color "$BASH" -c \
    "set -euo pipefail; source '$UI'; ui_say hello 2>&1 | cat -v"
  [[ "$output" == *"^[[38;5;"* ]] || { echo "CLICOLOR_FORCE painted nothing: $output"; false; }
}

@test "TERM=dumb is colourless even when colour is forced" {
  # There is no escape sequence a dumb terminal will not print at you literally.
  run env -u COLORTERM CLICOLOR_FORCE=1 TERM=dumb "$BASH" -c \
    "set -euo pipefail; source '$UI'; echo \$UI_PROFILE"
  [ "$output" = none ]
}

@test "the alphabet follows the locale, and the gutter is three cells either way" {
  run env LC_ALL=en_US.UTF-8 "$BASH" -c \
    "set -euo pipefail; source '$UI'; ui_glyph g say; printf '[%s]' \"\$g\"; ui_glyph g ok; printf '[%s]\n' \"\$g\""
  [ "$output" = '[🌫  ][✓  ]' ] || { echo "got: $output"; false; }

  # A C-locale terminal turns every mark into three question marks and shears
  # every column after it by two, so the ASCII alphabet is the honest answer.
  run env -u LANG -u LC_CTYPE LC_ALL=C "$BASH" -c \
    "set -euo pipefail; source '$UI'; ui_glyph g say; printf '[%s]' \"\$g\"; ui_glyph g ok; printf '[%s]\n' \"\$g\""
  [ "$output" = '[~  ][+  ]' ] || { echo "got: $output"; false; }
}

@test "a not-a-terminal live region prints one line per state change and no cursor escape" {
  ui_sh '
    ui_row run build 1s; ui_row wait test queued; ui_paint
    ui_row run build 2s; ui_row wait test queued; ui_paint
    ui_row ok  build 3s; ui_row ok   test 1s;     ui_paint'
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  # Four state changes across three frames: build+test starting, then each
  # finishing. The middle frame changed nothing and printed nothing.
  [ "${#lines[@]}" -eq 4 ] || { echo "got ${#lines[@]} lines: $output"; false; }
  [[ "$output" != *$'\033'* ]] || { echo "a cursor escape reached a pipe"; false; }
}

# ── the palette, against snug ────────────────────────────────────────────────

@test "every declared 256-colour index is the nearest one to its hex" {
  # The table in ui.sh is declared rather than searched at load time, which buys
  # ~2000 arithmetic iterations back from every script that sources it. This is
  # the price: the search, run here, against every entry.
  #
  # Written out again rather than imported from the generator on purpose — the
  # generator is the thing under test, and a test that asks it to check its own
  # arithmetic checks nothing. `dist` below is theme.go's, weights and all: 3/6/1
  # rather than plain Euclidean, because plain Euclidean picks a visibly
  # different neighbour for colours this low in chroma, and the fallback drawing
  # a different index from the binary is the one failure that must be impossible.
  run "$BASH" -c "set -euo pipefail; source '$UI'
    for k in \"\${!UI__HEX[@]}\"; do printf '%s %s %s\n' \"\$k\" \"\${UI__HEX[\$k]}\" \"\${UI__X256[\$k]}\"; done"
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  run python3 -c '
import sys
cube = [0, 95, 135, 175, 215, 255]
def dist(r1, g1, b1, r2, g2, b2):   # theme.go, weights and all
    dr, dg, db = r1-r2, g1-g2, b1-b2
    return 3*dr*dr + 6*dg*dg + db*db
def nearest(h):
    r, g, b = int(h[0:2], 16), int(h[2:4], 16), int(h[4:6], 16)
    best, bd = 0, 1 << 30
    for ri, rv in enumerate(cube):
        for gi, gv in enumerate(cube):
            for bi, bv in enumerate(cube):
                d = dist(r, g, b, rv, gv, bv)
                if d < bd: best, bd = 16 + 36*ri + 6*gi + bi, d
    for i in range(24):
        v = 8 + i*10
        d = dist(r, g, b, v, v, v)
        if d < bd: best, bd = 232 + i, d
    return best
bad = []
for line in sys.stdin.read().splitlines():
    if not line.strip(): continue
    key, hexv, declared = line.split()
    want = nearest(hexv)
    if want != int(declared): bad.append(f"{key}: #{hexv} declared {declared}, nearest is {want}")
print("\n".join(bad) if bad else "MATCHED")
' <<<"$output"
  [ "$output" = MATCHED ] || { echo "$output"; false; }
}

@test "the hex table matches the generated palette.go" {
  # The fallback drawing a DIFFERENT colour from the binary would be worse than
  # no fallback: it makes "which machine is this?" a question you have to ask
  # about your own output.
  #
  # One run of `script/gen-palette.sh` writes both files, so this can now only
  # fail two ways — someone hand-edited between ui.sh's markers, or the two
  # halves were regenerated from different nebelung revs — and both are exactly
  # what it is here to catch. No skip and no `SNUG_SRC` escape hatch: while this
  # test lived in the workshop it needed a snug checkout beside it and went
  # quietly green on any machine without one, which is half of why it moved.
  local pal="$BATS_TEST_DIRNAME/../palette.go"
  [ -r "$pal" ] || { echo "no palette.go beside share/ui.sh (looked in $pal)"; false; }

  run "$BASH" -c "set -euo pipefail; source '$UI'
    for k in \"\${!UI__HEX[@]}\"; do printf '%s %s\n' \"\$k\" \"\${UI__HEX[\$k]}\"; done | sort"
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  local ours="$output"

  # palette.go is `Variant: { "token": "#hex", … }`, one variant per block, in
  # the same order as the Variant constants. Parsed rather than diffed against a
  # golden file so that gofmt re-spacing the map literal is not a test failure.
  run python3 -c '
import re, sys
src = open(sys.argv[1]).read()
names = {"Nebelung": "nebelung", "NebelungHighContrast": "nebelung-high-contrast",
         "NebelungLatte": "nebelung-latte", "NebelungLatteHC": "nebelung-latte-high-contrast"}
out = []
for m in re.finditer(r"(\w+):\s*\{(.*?)\n\t\}", src, re.S):
    variant = names.get(m.group(1))
    if not variant: continue
    for tok, hexv in re.findall(r"\"(\w+)\":\s*\"#([0-9a-fA-F]{6})\"", m.group(2)):
        out.append(f"{variant}:{tok} {hexv.lower()}")
print("\n".join(sorted(out)))
' "$pal"
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  [ "$ours" = "$output" ] || {
    echo "share/ui.sh and palette.go disagree — regenerate both with script/gen-palette.sh."
    diff <(echo "$ours") <(echo "$output") || true
    false
  }
}

# ── the width probe ──────────────────────────────────────────────────────────

@test "ui_measure survives no tty and no TERM instead of killing its caller" {
  # The measured failure, and it shipped in `bench` and `haus.sh` both: `set -e`
  # exempts every command in a `&&`/`||` list EXCEPT the last, and `tput` exits
  # 2 with TERM unset. Without a `|| true` inside that final substitution the
  # caller exits 2 with nothing on either stream, after a successful evaluation
  # and before anything activated — and the sanitising `case` whose entire job
  # is to cope with a bad answer never runs.
  #
  # No pty on purpose: this is the shape of `ssh mac bench rebuild`, a launchd
  # job and CI, which is exactly where nobody is watching to notice.
  run env -u TERM -u COLUMNS "$BASH" -c \
    "set -euo pipefail; source '$UI'; ui_measure; echo COLS=\$UI_COLS" </dev/null
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  # No window at all, so nothing is folded — the pipe is not given an invented
  # geometry, which is the `tput cols` mistake one layer up.
  [[ "$output" == *"COLS=1048576"* ]] || { echo "got: $output"; false; }
}

@test "ui_measure reads COLUMNS from the kernel, not rows and not terminfo" {
  # Two ways to get this wrong, and both look fine in a source grep:
  #   · `tput cols` reads terminfo's STATIC size — 80 for every xterm-* entry —
  #     and answers 80 in a 37-column window. Only TIOCGWINSZ tracks a resize.
  #   · `stty size` prints "<rows> <cols>", so a `${sz% *}` takes the ROWS.
  # 24×37: neither field is 80 and neither equals the other, so a pty is the
  # only thing that can tell the three answers apart.
  run python3 - "$UI" "$BASH" <<'PYEOF'
import os, pty, sys, fcntl, termios, struct, select
ui, bash = sys.argv[1], sys.argv[2]
pid, fd = pty.fork()
if pid == 0:
    # Gate on a read so the parent's TIOCSWINSZ lands before we measure.
    os.execvp(bash, [bash, "-c",
        f"read -r _; source {ui}; ui_measure; echo COLS=$UI_COLS AVAIL=$UI_AVAIL"])
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 37, 0, 0))
os.write(fd, b"\n")
out = b""
while True:
    r, _, _ = select.select([fd], [], [], 5)
    if not r:
        break
    try:
        d = os.read(fd, 65536)
    except OSError:
        break
    if not d:
        break
    out += d
    if b"COLS=" in out:
        break
sys.stdout.write(out.decode("utf8", "replace"))
PYEOF
  [[ "$output" == *"COLS=37"* ]] || { echo "got: $output"; false; }
  # And the last column is never written into.
  [[ "$output" == *"AVAIL=36"* ]] || { echo "got: $output"; false; }
}

# ── living beside a caller ───────────────────────────────────────────────────

@test "a live region gives the caller's EXIT trap back" {
  # A library that takes EXIT eats whatever cleanup the script had, and in this
  # family that is a scratch worktree or a temp file. It has to chain, not
  # clobber — and it has to put the trap back when the region closes.
  run python3 - "$UI" "$BASH" <<'PYEOF'
import os, pty, sys, select
ui, bash = sys.argv[1], sys.argv[2]
script = (
    f"source {ui}; "
    "trap 'echo CALLER-CLEANUP-RAN' EXIT; "
    "ui_row ok build 1s; ui_paint; ui_live_close; "
    "echo DONE"
)
pid, fd = pty.fork()
if pid == 0:
    os.execvp(bash, [bash, "-c", script])
out = b""
while True:
    r, _, _ = select.select([fd], [], [], 5)
    if not r: break
    try: d = os.read(fd, 65536)
    except OSError: break
    if not d: break
    out += d
sys.stdout.write(out.decode("utf8", "replace"))
PYEOF
  [[ "$output" == *"CALLER-CLEANUP-RAN"* ]] || { echo "the caller's EXIT trap was eaten: $output"; false; }
  # And the cursor came back on, which is the worst thing to leave behind.
  [[ "$output" == *$'\033[?25h'* ]] || { echo "the cursor was never restored: $(printf %q "$output")"; false; }
}

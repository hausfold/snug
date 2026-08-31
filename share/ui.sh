#!/usr/bin/env bash
# ui.sh — snug's bash half, for a machine with no `snug` on PATH.
#
# The standard (hausfold/workshop's docs/cli-presentation.md) ships as two
# implementations of one spec, because bash cannot link a Go library. Both live
# in THIS repo, which is what keeps them one thing:
#
#   the Go package        the library, and the `snug` binary shell callers drive
#   share/ui.sh           the same spec, at lower fidelity, in pure bash
#
# When does the fallback run? An older generation still on someone's disk, a
# script invoked off a thin PATH (a launchd job, a `ssh mac bench …` with no
# login shell), a `bench` checkout on a Mac that never installed the layer, and
# CI. On those, this is the whole painter — so it has to be honest on its own,
# not a stub that assumes the real thing is one directory away.
#
# ── how a caller reaches it ──────────────────────────────────────────────────
# It is installed at `$out/share/ui.sh`, beside `bin/snug`, so a Nix consumer
# reads it off the store path with no vendored copy and no drift check:
#
#   # in a derivation, where `snug` is `pkgs.snug` from this flake's overlay
#   --set HAUS_UI_SH ${snug}/share/ui.sh
#
#   # in a checkout of the family, where `bench` knows where snug lives
#   source "$(repo_dir snug)/share/ui.sh"
#
# Either way the caller must DEGRADE when it is missing rather than assume it —
# the same contract `haus-notify` keeps toward trill.
#
#   ui_say "resolving inputs"          # → stderr, folded, hung at the gutter
#   ui_data "$path"                    # → stdout, untouched
#   ui_row run build "12s"; ui_paint   # → a live region, repainted in place
#   ui_live_close
#
# ── how this stays in step with the binary ───────────────────────────────────
# Every number here is the Go side's number, and the comments say so where it
# matters. The palette below is GENERATED: `script/gen-palette.sh` writes the
# `UI__HEX` and `UI__X256` blocks and `palette.go` from the same nebelung
# checkout and the same `TOKENS` list, in one run, so the two cannot disagree by
# hand. `test/ui.bats` re-derives every 256-colour index from the hex and diffs
# the hex against `palette.go`. A fallback that drew a DIFFERENT line from the
# binary would be worse than no fallback at all — it would make "which machine
# is this?" a question you have to ask about your own output.
#
# ⚠️ Deliberately NOT a wrapper around `snug` when snug happens to be present.
# The dispatch belongs to the caller: it opens ONE coprocess for a whole command
# (a fork is ~4.5 ms, and sixty forks is a third of a second of nothing), and a
# library sourced per script cannot see the command boundary that decision needs.
#
# Bash 4+ — associative arrays, `${var,,}`, `printf -v`. Every family CLI
# already requires it (`declare -gA`, `$EPOCHSECONDS`), and macOS's /bin/bash
# 3.2 is not what any of them run under.

# Sourced more than once — bench sources this, and the bats harness sources
# bench — is a no-op, not a re-detection: `ui_measure` is the only thing that
# should ever re-read the window.
[ -n "${UI_SH:-}" ] && return 0
UI_SH=1

# ── what the far end can do ──────────────────────────────────────────────────
# Measured from STDERR, not stdout, and that is the family stream contract
# rather than a preference: stdout carries DATA only, so callers can do
# `cd "$(scruff child …)"`. The human stream is fd 2, so fd 2 is what we ask
# about the window, the colour and the cursor. A tool whose prose is redirected
# to a file while the terminal is still there gets a painter, which is right.
UI_TTY=""; [ -t 2 ] && UI_TTY=1

# And the OTHER stream, because a REPORT lands on it. `bench status`'s tables
# and the `scruff` listing are the thing the user ran the command for rather
# than the tool talking about it, so they draw on fd 1 — and a report is
# measured, gated and painted for the stream it lands on, never the other one.
# The two questions come apart in both directions: budget a report from stderr
# and a TTY stdout beside a redirected stderr draws plain; gate it from stderr
# and a piped stdout beside a live stderr draws escapes into the pipe.
#
# `ui_table_data` is the only thing here that asks this one. Everything else in
# this file is the tool TALKING, and talking is fd 2.
UI_OUT_TTY=""; [ -t 1 ] && UI_OUT_TTY=1

# UI_PROFILE — none | 16 | 256 | truecolor
#
# The precedence is the one every well-behaved CLI agrees on and is easy to get
# subtly wrong: NO_COLOR beats everything except CLICOLOR_FORCE, and a non-TTY
# is colourless unless forced. https://no-color.org · https://bixense.com/clicolors
#
# ui__detect_profile [istty] [outvar] — both default to fd 2's answer, which is
# what every verb in this file draws with. `ui_table_data` needs the identical
# question asked about fd 1, and one function has to answer both or the two
# gates drift: the environment half (NO_COLOR, CLICOLOR_FORCE, TERM=dumb) is
# the same for every stream and only the terminal-ness differs.
ui__detect_profile() {
  local forced="" t tty="${1-$UI_TTY}" out="${2:-UI_PROFILE}"
  case "${CLICOLOR_FORCE:-}" in '' | 0) ;; *) forced=1 ;; esac
  if [ -n "${NO_COLOR+set}" ] && [ -z "$forced" ]; then printf -v "$out" '%s' none; return 0; fi
  if [ -z "$tty" ] && [ -z "$forced" ]; then printf -v "$out" '%s' none; return 0; fi
  t="${TERM:-}"
  # "dumb" means it, and it means it under CLICOLOR_FORCE too: there is no
  # escape sequence a dumb terminal will not print at you literally.
  if [ "$t" = dumb ]; then printf -v "$out" '%s' none; return 0; fi
  case "${COLORTERM:-}" in
    truecolor | 24bit | TRUECOLOR | 24BIT) printf -v "$out" '%s' truecolor; return 0 ;;
  esac
  case "$t" in
    *truecolor* | *direct*) printf -v "$out" '%s' truecolor ;;
    *256*) printf -v "$out" '%s' 256 ;;
    # Forced, with nothing to go on — a CI log renderer, usually. 256 is the
    # safe middle: universally understood, and a small step from the hex.
    '') printf -v "$out" '%s' 256 ;;
    *) printf -v "$out" '%s' 16 ;;
  esac
  return 0
}
ui__detect_profile
ui__detect_profile "$UI_OUT_TTY" UI_OUT_PROFILE

# UI_VARIANT — which nebelung this machine is wearing. An explicit override,
# then the file haus writes when it resolves haus.theme.flavor. Neither present
# means the default dark, which is also the honest answer on a machine with no
# haus at all.
ui__detect_variant() {
  local v="" f
  v="${SNUG_VARIANT:-}"
  if [ -z "$v" ]; then
    f="${XDG_CONFIG_HOME:-$HOME/.config}/snug/variant"
    [ -r "$f" ] && read -r v <"$f" 2>/dev/null
  fi
  v="${v,,}"; v="${v//[[:space:]]/}"
  case "$v" in
    nebelung | mocha) UI_VARIANT=nebelung ;;
    nebelung-high-contrast) UI_VARIANT=nebelung-high-contrast ;;
    nebelung-latte | latte) UI_VARIANT=nebelung-latte ;;
    nebelung-latte-high-contrast) UI_VARIANT=nebelung-latte-high-contrast ;;
    *) UI_VARIANT=nebelung ;;
  esac
  return 0
}
ui__detect_variant

# UI_UNICODE — may we use the UTF-8 alphabet?
#
# The locale is the only signal there is, and it is a weak one: a UTF-8 locale
# says nothing about which glyphs the FONT has. But the failure it prevents is
# the loud one — mojibake in a C-locale terminal, where every mark becomes three
# question marks and every column after it shears by two.
ui__detect_alphabet() {
  local k v
  case "${SNUG_ASCII:-}" in '' | 0) ;; *) UI_UNICODE=""; return 0 ;; esac
  for k in LC_ALL LC_CTYPE LANG; do
    v="${!k:-}"
    [ -z "$v" ] && continue
    case "${v^^}" in *UTF-8* | *UTF8*) UI_UNICODE=1 ;; *) UI_UNICODE="" ;; esac
    return 0
  done
  UI_UNICODE=""
  return 0
}
ui__detect_alphabet

# ── the palette ──────────────────────────────────────────────────────────────
# Nine roles. A tool names a role, never a colour — that indirection is the
# whole point. Seven hand-picked 256-colour indices, copy-pasted into four
# files, is how the family ended up ΔE 2–27 from the flavour every other tool on
# this machine was wearing, with two different greys for one role and a primary
# accent that resolved to BLUE, the one hue nebelung exists to strip out.
#
# Both tables below are GENERATED, by `script/gen-palette.sh`, from the same
# nebelung checkout and the same TOKENS list that writes `palette.go` — one run,
# two files, so the fallback and the binary cannot drift apart by hand. That is
# what moving this file into snug bought; do not hand-edit between the markers.
#
# The 256-colour index beside each hex is DECLARED, not searched at load time.
# It is `nearest256` from theme.go, ported into the generator digit for digit —
# the closest of xterm's 216-colour cube and its 24-step grey ramp, searched
# rather than divided out because the cube's levels (0 95 135 175 215 255) are
# not evenly spaced, and weighted 3/6/1 because plain Euclidean RGB picks
# visibly wrong neighbours for colours this low in chroma. Declaring it saves
# ~2000 arithmetic iterations in every script that sources this, for an answer
# that can only change when the palette does; `test/ui.bats` pays the price by
# re-deriving every one of them.
#
# ⚠️ `sapphire` (#7dc6e7) and `teal` (#9be0d5) both resolve to 116 on the dark
# variants, so `subject` and `path` are the same colour at 256. That is the
# palette's arithmetic, not a typo here — snug's binary does the same thing, and
# the fallback matching it is the point. Worth fixing in nebelung's tokens or in
# snug's roleToken if it ever reads as a bug; do not fix it only here.
# ▼▼▼ generated by script/gen-palette.sh — DO NOT EDIT ▼▼▼
declare -gA UI__HEX=(
  [nebelung:mauve]=c9a8f1     [nebelung:green]=abe1a6
  [nebelung:peach]=f5b58e     [nebelung:red]=ed8fa9
  [nebelung:overlay1]=858585  [nebelung:sapphire]=7dc6e7
  [nebelung:teal]=9be0d5      [nebelung:subtext0]=aeaeae

  [nebelung-high-contrast:mauve]=c9a8f1     [nebelung-high-contrast:green]=abe1a6
  [nebelung-high-contrast:peach]=f5b58e     [nebelung-high-contrast:red]=ed8fa9
  [nebelung-high-contrast:overlay1]=8e8e8e  [nebelung-high-contrast:sapphire]=7dc6e7
  [nebelung-high-contrast:teal]=9be0d5      [nebelung-high-contrast:subtext0]=c6c6c6

  [nebelung-latte:mauve]=8545e3     [nebelung-latte:green]=4a9e3a
  [nebelung-latte:peach]=f66d2d     [nebelung-latte:red]=ca2a40
  [nebelung-latte:overlay1]=909090  [nebelung-latte:sapphire]=379eb1
  [nebelung-latte:teal]=2f9197      [nebelung-latte:subtext0]=717171

  [nebelung-latte-high-contrast:mauve]=8545e3     [nebelung-latte-high-contrast:green]=4a9e3a
  [nebelung-latte-high-contrast:peach]=f66d2d     [nebelung-latte-high-contrast:red]=ca2a40
  [nebelung-latte-high-contrast:overlay1]=8d8d8d  [nebelung-latte-high-contrast:sapphire]=379eb1
  [nebelung-latte-high-contrast:teal]=2f9197      [nebelung-latte-high-contrast:subtext0]=686868
)
declare -gA UI__X256=(
  [nebelung:mauve]=183     [nebelung:green]=151
  [nebelung:peach]=216     [nebelung:red]=211
  [nebelung:overlay1]=102  [nebelung:sapphire]=116
  [nebelung:teal]=116      [nebelung:subtext0]=145

  [nebelung-high-contrast:mauve]=183     [nebelung-high-contrast:green]=151
  [nebelung-high-contrast:peach]=216     [nebelung-high-contrast:red]=211
  [nebelung-high-contrast:overlay1]=245  [nebelung-high-contrast:sapphire]=116
  [nebelung-high-contrast:teal]=116      [nebelung-high-contrast:subtext0]=251

  [nebelung-latte:mauve]=98      [nebelung-latte:green]=71
  [nebelung-latte:peach]=202     [nebelung-latte:red]=161
  [nebelung-latte:overlay1]=246  [nebelung-latte:sapphire]=73
  [nebelung-latte:teal]=30       [nebelung-latte:subtext0]=242

  [nebelung-latte-high-contrast:mauve]=98      [nebelung-latte-high-contrast:green]=71
  [nebelung-latte-high-contrast:peach]=202     [nebelung-latte-high-contrast:red]=161
  [nebelung-latte-high-contrast:overlay1]=245  [nebelung-latte-high-contrast:sapphire]=73
  [nebelung-latte-high-contrast:teal]=30       [nebelung-latte-high-contrast:subtext0]=242
)
# ▲▲▲ end generated ▲▲▲

# role → nebelung token. `body` is deliberately absent rather than mapped to
# `text`: painting ordinary prose fights the reader's own background and is the
# single fastest way to look cheap.
declare -gA UI__TOKEN=(
  [accent]=mauve   [ok]=green      [warn]=peach  [err]=red
  [muted]=overlay1 [subject]=sapphire [path]=teal [field]=subtext0
)

# What a role becomes at sixteen colours, DECLARED rather than computed.
#
# Nearest-RGB is the wrong algorithm here and measurably so: nebelung is pastel,
# so `green` (#abe1a6) and `peach` (#f5b58e) both land nearer mid-grey than any
# named colour, and `ok` and `warn` come out identical — the two roles it
# matters most to tell apart. Sixteen colours are NAMES, so this maps by intent:
# ok is green because ok means green, not because the arithmetic said so.
#
# The roles the sixteen cannot carry collapse on purpose, rather than two of
# them landing on one base colour by accident and reading as a bug.
declare -gA UI__ANSI16=(
  [accent]=95  # bright magenta — mauve's nearest name
  [ok]=92      # bright green
  [warn]=93    # bright yellow
  [err]=91     # bright red
  [muted]=90   # bright black, which every theme renders as a grey
  [field]=37   # white
  [subject]=95 # collapses to accent
  [path]=90    # collapses to muted
)

# Every role resolved once, into UI_<ROLE>. Colour must live OUTSIDE a width —
# an escape counted as width shears every column after it — so these are only
# ever wrapped AROUND a pre-padded field, never inside a `%-*s`.
#
# ui__resolve_palette [profile] [prefix] — both default to fd 2's, the stream
# every verb here draws on. The second call below resolves the SAME variant at
# fd 1's profile into `UI_OUT_*`, because a report is painted for the stream it
# lands on: one palette, two gates.
ui__resolve_palette() {
  local role tok hex r g b sgr prof="${1:-$UI_PROFILE}" pfx="${2:-UI_}"
  sgr=""; [ "$prof" != none ] && sgr=$'\033[0m'
  printf -v "${pfx}OFF" '%s' "$sgr"
  for role in accent ok warn err muted subject path field; do
    sgr=""
    tok="${UI__TOKEN[$role]}"
    case "$prof" in
      none) ;;
      16) sgr=$'\033['"${UI__ANSI16[$role]}"'m' ;;
      256) sgr=$'\033[38;5;'"${UI__X256[$UI_VARIANT:$tok]}"'m' ;;
      truecolor)
        hex="${UI__HEX[$UI_VARIANT:$tok]}"
        r=$(( 16#${hex:0:2} )); g=$(( 16#${hex:2:2} )); b=$(( 16#${hex:4:2} ))
        sgr=$'\033[38;2;'"$r;$g;${b}"'m'
        ;;
    esac
    printf -v "${pfx}${role^^}" '%s' "$sgr"
  done
  # `body` is ordinary text and stays the terminal's own colour, always. It is
  # part of the public nine even though it is always empty — a caller naming
  # `body` is saying "deliberately unpainted", which is worth being able to say.
  printf -v "${pfx}BODY" '%s' ""
  return 0
}
ui__resolve_palette
ui__resolve_palette "$UI_OUT_PROFILE" UI_OUT_

# ui_paint_role <var> <role> <text> [prefix] — <text> wrapped in a role, into
# <var>. The prefix picks which stream's palette: `UI_` (fd 2, the default) or
# `UI_OUT_` (fd 1, what a report is painted with).
#
# `printf -v` rather than `$( )` because a live region calls this ten times a
# second per row: a command substitution there was thirty forks a second in a
# loop whose own comment said everything in it was a builtin.
ui_paint_role() {
  local pfx="${4:-UI_}" sgr off
  sgr="${pfx}${2^^}"; sgr="${!sgr:-}"
  off="${pfx}OFF";    off="${!off:-}"
  if [ -z "$sgr" ]; then printf -v "$1" '%s' "$3"
  else printf -v "$1" '%s%s%s' "$sgr" "$3" "$off"; fi
  return 0
}

# ── the glyphs ───────────────────────────────────────────────────────────────
# The glyph is load-bearing and the colour is not: every role has to survive
# NO_COLOR, so the glyph carries the meaning and the colour is the courtesy.
#
# Widths are DECLARED, and that is the important part. No mark defaults to emoji
# presentation — but that alone does not make measurement safe, because the
# hazard is East_Asian_Width. `ⓘ` (U+24D8), `–` (U+2013), `·` (U+00B7) and the
# truncation `…` (U+2026) are Ambiguous: one cell in a Western locale, TWO under
# an East-Asian one, and every width library has a mode for each. `⚠` (U+26A0)
# is the other shape — Emoji = Yes, one cell bare, two as the emoji-presentation
# sequence U+26A0 U+FE0F, which is why the table holds bare codepoints and never
# a sequence. These match snug's glyph.go, verified against a real terminal. To
# re-check one, and get a number rather than an eyeball:
#
#   stty -echo; printf '\r\u24D8\033[6n'; IFS=';' read -rd R _ col; stty echo
#   echo "$col"   # 2 → one cell, 3 → two
declare -gA UI__GLYPH_UTF8=(
  [say]=$'\u224B' [ok]='✓' [warn]='⚠' [err]='✗'
  [info]='ⓘ' [skip]='–' [bullet]='·' [hint]='↳'
)
declare -gA UI__GLYPH_ASCII=(
  [say]='~' [ok]='+' [warn]='!' [err]='x'
  [info]='i' [skip]='-' [bullet]='.' [hint]='>'
)
# Every mark is one cell, including every spinner frame, and the gutter budgets
# on that — a future glyph wider than one cell has to be paired with a change to
# UI_GUTTER.
UI_GUTTER=3

UI__SPIN_UTF8=(⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏)
UI__SPIN_ASCII=('|' '/' '-' $'\\')

# ui_glyph <var> <mark> — the mark, padded to the gutter, into <var>.
#
# Padding here rather than at the call site is what keeps a ✓ line and a ≋ line
# starting their text in the same column.
ui_glyph() {
  local __ui_g
  ui_glyph_bare __ui_g "$2"
  if [ -z "$__ui_g" ]; then printf -v "$1" '%*s' "$UI_GUTTER" ''; return 0; fi
  printf -v "$1" '%s%*s' "$__ui_g" $(( UI_GUTTER - 1 )) ''
  return 0
}

# ui_glyph_bare <var> <mark> — the mark ALONE, one cell, no padding.
#
# The narrow tiers need this and the padded form would be a bug there: at `bare`
# the whole gutter has collapsed to a single space, so a mark still carrying two
# cells of padding puts the row two cells past the window's edge. It is invisible
# with colour off — a trailing-space trim hides it — which is exactly how it
# survived in snug until this file was written against it.
ui_glyph_bare() {
  local g
  if [ -n "$UI_UNICODE" ]; then g="${UI__GLYPH_UTF8[$2]:-}"; else g="${UI__GLYPH_ASCII[$2]:-}"; fi
  printf -v "$1" '%s' "$g"
  return 0
}

# ui_spin <var> <frame> — one spinner frame, padded to the gutter.
ui_spin() {
  local __ui_s
  ui_spin_bare __ui_s "$2"
  printf -v "$1" '%s%*s' "$__ui_s" $(( UI_GUTTER - 1 )) ''
  return 0
}

# ui_spin_bare <var> <frame> — one spinner frame alone, one cell.
ui_spin_bare() {
  local n="$2"
  [ "$n" -lt 0 ] && n=$(( -n ))
  if [ -n "$UI_UNICODE" ]; then
    printf -v "$1" '%s' "${UI__SPIN_UTF8[$(( n % ${#UI__SPIN_UTF8[@]} ))]}"
  else
    printf -v "$1" '%s' "${UI__SPIN_ASCII[$(( n % ${#UI__SPIN_ASCII[@]} ))]}"
  fi
  return 0
}

# The cut mark. One cell in both alphabets.
UI_ELLIPSIS='~'; [ -n "$UI_UNICODE" ] && UI_ELLIPSIS='…'

# ── measuring the window ─────────────────────────────────────────────────────
# NOT `tput cols`. terminfo carries a STATIC size — 80 for every xterm-* entry —
# and ncurses only overrides it from the real window when LINES/COLUMNS are
# exported. Measured in a 40-column pty, `tput cols` answers 80, so a painter
# built on it folds to a width the window has not had since 1978 and looks
# exactly like a painter that works, right up until someone narrows a terminal.
# `stty size` asks the kernel (TIOCGWINSZ), the only source that tracks a resize.
#
# It reads /dev/tty rather than `<&1` because this runs inside `$( )`, where fd 1
# is the substitution's own pipe and never the terminal.
#
# ⚠️ `|| true` inside that LAST substitution is load-bearing under `set -e`, not
# defensive. Every command in a `&&`/`||` list is exempt from `set -e` EXCEPT the
# final one, and `tput` exits 2 with TERM unset — the exact shape of a session
# with no pty (`ssh mac bench rebuild`, a launchd job, CI). Both `bench` and
# `haus.sh` shipped this line without it and both were measured killing their
# caller: exit 2, nothing on either stream, after a successful evaluation and
# before anything activated, with the sanitising `case` that exists precisely to
# cope with a bad answer never running at all. A width probe that can kill the
# caller it was being polite to is the worst possible failure mode for a
# courtesy — and is the whole argument for one runtime instead of four copies.
#
# `stty` forks, so this runs once at startup and again only when SIGWINCH says
# the window actually changed. Never per line, and never per frame.
UI_CAP=100        # past this a line of prose is unreadable, and a maximised
                  # terminal is not an invitation to fill it
UI_NOFOLD=1048576 # the width of a stream with no window: wide enough that
                  # nothing is ever folded, finite so the arithmetic stays plain
UI_COLS=80; UI_AVAIL=79; UI_PROSE=80; UI_OUT_AVAIL=79; UI_RESIZED=0

ui_measure() {
  local sz
  # A stream that is NOT a terminal is never folded, and that is this library's
  # own principle rather than a special case: `tput cols` is wrong because it
  # answers a static 80 for a window it never measured, and assuming 80 for a
  # pipe is the identical mistake one layer up. A redirected stream has no width
  # to fit, and whatever is on the far end is usually grepping for whole lines.
  #
  # Neither stream on a terminal means there is no window to ask about at all,
  # and `stty` is not worth the fork.
  if [ -z "$UI_TTY" ] && [ -z "$UI_OUT_TTY" ]; then
    UI_COLS="$UI_NOFOLD"; UI_AVAIL="$UI_NOFOLD"; UI_PROSE="$UI_NOFOLD"
    UI_OUT_AVAIL="$UI_NOFOLD"
    return 0
  fi
  sz="$(stty size 2>/dev/null </dev/tty)" && UI_COLS="${sz#* }" \
    || UI_COLS="$(tput cols 2>/dev/null || true)"
  case "$UI_COLS" in '' | *[!0-9]*) UI_COLS="${COLUMNS:-80}" ;; esac
  case "$UI_COLS" in '' | *[!0-9]*) UI_COLS=80 ;; esac
  [ "$UI_COLS" -lt 1 ] && UI_COLS=1
  # Never write into the last column: a line whose width EQUALS the terminal's
  # leaves the cursor past the edge and the terminal wraps it anyway. That is
  # the off-by-one every live region in this family broke on.
  UI_AVAIL=$(( UI_COLS - 1 )); [ "$UI_AVAIL" -lt 1 ] && UI_AVAIL=1
  # Prose gets the CAP, and deliberately the full width rather than UI_AVAIL —
  # this is snug's `Prose()` arithmetic, matched on purpose so the two painters
  # fold at the same column. (Tables and live regions are exempt from the cap:
  # they are bounded by their own content, and a job list stopping at 100 in a
  # 200-column window would be hiding the room it had.)
  UI_PROSE="$UI_COLS"; [ "$UI_PROSE" -gt "$UI_CAP" ] && UI_PROSE="$UI_CAP"
  UI_OUT_AVAIL="$UI_AVAIL"
  # One window, measured once; each stream then gets the answer for ITS OWN far
  # end. `/dev/tty` is the controlling terminal, so the measurement above is
  # valid whichever of the two is the terminal — but a redirected stream still
  # has no width, and giving it the window's is the `tput cols` mistake wearing
  # the other stream's clothes. `bench status > report.txt` on a live terminal
  # writes the report whole; `bench status 2>/dev/null` still fits its table.
  [ -z "$UI_TTY" ] && { UI_AVAIL="$UI_NOFOLD"; UI_PROSE="$UI_NOFOLD"; }
  [ -z "$UI_OUT_TTY" ] && UI_OUT_AVAIL="$UI_NOFOLD"
  UI_RESIZED=1
  return 0
}
ui_measure
UI_RESIZED=0

# ── text ─────────────────────────────────────────────────────────────────────
# Width here is CHARACTERS (`${#s}` under a UTF-8 locale), not cells, and that
# is this file's one real concession to being bash: our own glyphs carry
# declared widths, and everything else it measures — repo names, durations,
# labels, paths — is text where the two numbers are equal. The day a CALLER
# hands it an emoji, a snug binary on PATH is the answer, not an arithmetic
# patch here. What it must never do is count BYTES: `printf '%-9s'` pads by
# bytes under most locales, which is how every column after a multi-byte glyph
# ended up sheared by a different amount depending on which glyph it was.
#
# Text handed to these carries no escapes. Colour goes on afterwards, around a
# finished field, for the same reason.

# ui_truncate <var> <text> <width> [tail] — <text> cut to <width>, cut mark
# included. The tail defaults to the ellipsis; pass '' for a silent cut, which
# is what a LABEL wants — a truncated column head has no room to also carry a
# mark saying so.
#
# The mark is INSIDE the budget, so the result is never wider than asked: the
# caller has already spent those cells on something else.
ui_truncate() {
  local s="$2" w="$3" tail="${4-$UI_ELLIPSIS}" keep
  if [ "$w" -le 0 ]; then printf -v "$1" '%s' ''; return 0; fi
  if [ "${#s}" -le "$w" ]; then printf -v "$1" '%s' "$s"; return 0; fi
  keep=$(( w - ${#tail} )); [ "$keep" -lt 0 ] && keep=0
  printf -v "$1" '%s%s' "${s:0:$keep}" "$tail"
  return 0
}

# ui_truncate_left <var> <text> <width> [head] — cut from the FRONT, which is
# what a path wants: `…/internal/ui` keeps the part that identifies the file,
# where cutting the other end leaves every path in a repo looking identical.
#
# It cuts at a separator when it can. `…/orkshop/haus` is worse than useless —
# it reads as a directory that does not exist, and the eye stops on it.
ui_truncate_left() {
  local s="$2" w="$3" head="${4-$UI_ELLIPSIS}" hw len keep best sep i
  if [ "$w" -le 0 ]; then printf -v "$1" '%s' ''; return 0; fi
  len="${#s}"
  if [ "$len" -le "$w" ]; then printf -v "$1" '%s' "$s"; return 0; fi
  hw="${#head}"
  if [ "$hw" -ge "$w" ]; then ui_truncate "$1" "$head" "$w" ''; return 0; fi
  keep=$(( w - hw ))
  # Width here is characters, so "the suffix from i still fits" is `len - i <=
  # keep` and the leftmost cut that fits is arithmetic rather than a walk. The
  # separator is INCLUDED (`…/haus`, not `…haus`), so the first one at or after
  # that position is the longest separator-cut that fits — taking the one before
  # it on trust returns a string one cell wider than asked for, which is the
  # off-by-one this whole library exists to stop.
  best=$(( len - keep )); sep=-1
  for (( i = best; i < len; i++ )); do
    if [ "${s:i:1}" = / ]; then sep="$i"; break; fi
  done
  # Prefer the separator whenever it keeps at least half of what the raw cut
  # would; below that the boundary costs more than it earns.
  if [ "$sep" -ge 0 ] && [ $(( (len - sep) * 2 )) -ge $(( len - best )) ]; then
    printf -v "$1" '%s%s' "$head" "${s:sep}"
  else
    printf -v "$1" '%s%s' "$head" "${s:best}"
  fi
  return 0
}

# ui_pad <var> <text> <width> — <text> padded to <width> CHARACTERS.
#
# Not `printf '%-*s'`, which pads by BYTES: a cell holding `└` came up two
# short under every UTF-8 locale, and every column after it sheared by a
# different amount depending on which glyph was in front of it. `bench` carried
# two hand-padded cells to work around exactly that.
ui_pad() {
  local d=$(( $3 - ${#2} ))
  if [ "$d" -gt 0 ]; then printf -v "$1" '%s%*s' "$2" "$d" ''
  else printf -v "$1" '%s' "$2"; fi
  return 0
}

# ui_fold <width> <text> — folded lines into the UI_FOLD array.
#
# The family never soft-wraps. A tool that lets the terminal wrap has given up
# its own indentation — and inside a live region it has desynced the repaint,
# because the cursor moves up by the number of lines PRINTED and the terminal
# counted the ones it drew.
ui_fold() {
  local w="$1" para
  [ "$w" -lt 1 ] && w=1
  UI_FOLD=()
  while IFS= read -r para || [ -n "$para" ]; do
    ui__fold_one "$w" "$para"
  done <<<"$2"
  return 0
}

ui__fold_one() {
  local w="$1" line="" word
  local -a words=()
  # Deliberate word splitting: folding happens AT whitespace, so the split is
  # the algorithm and not an accident.
  # shellcheck disable=SC2206
  words=( $2 )
  if [ "${#words[@]}" -eq 0 ]; then UI_FOLD+=(""); return 0; fi
  for word in "${words[@]}"; do
    # Longer than a whole line on its own — a store path, a URL. Take a full
    # line of it and carry the rest: overflowing is the one thing never allowed.
    while [ "${#word}" -gt "$w" ]; do
      [ -n "$line" ] && { UI_FOLD+=("$line"); line=""; }
      UI_FOLD+=("${word:0:$w}")
      word="${word:$w}"
    done
    if [ -z "$line" ]; then line="$word"
    elif [ $(( ${#line} + 1 + ${#word} )) -le "$w" ]; then line="$line $word"
    else UI_FOLD+=("$line"); line="$word"
    fi
  done
  [ -n "$line" ] && UI_FOLD+=("$line")
  return 0
}

# ── the six verbs ────────────────────────────────────────────────────────────
# Every one of them writes to STDERR. Stdout carries data only — `ui_data` is
# the single thing in this file that touches it — because callers do
# `cd "$(scruff child …)"` and hooks read paths off fd 1. That contract is also
# why `snug run` works as a bash coproc: bash pipes stdin and stdout and leaves
# stderr on the terminal, which is exactly this split.
ui__line() { # ui__line <mark> <role> <text>
  local mark budget gut head pad i=0 painted
  # The gutter is fixed at three cells whatever the mark is, so a ✓ line and a
  # ≋ line start their text in the same column and a folded continuation has
  # somewhere to hang from.
  #
  # Under four columns it cannot be: three cells of gutter plus one of text is
  # already past the edge. So it collapses to the mark and a single space — the
  # same floor the live region's `bare` tier drops to, for the same reason.
  # Clamping the text budget to 1 while the gutter stayed at 3 is how a
  # two-column window got a four-cell line.
  if [ "$UI_PROSE" -ge $(( UI_GUTTER + 1 )) ]; then gut="$UI_GUTTER"  # mark, padded
  elif [ "$UI_PROSE" -ge 3 ]; then gut=2                              # mark and a space
  else gut=1                                                          # mark, nothing else
  fi
  budget=$(( UI_PROSE - gut ))
  if [ "$gut" -eq "$UI_GUTTER" ]; then
    ui_glyph mark "$1"
  else
    ui_glyph_bare mark "$1"
    [ "$gut" -eq 2 ] && printf -v mark '%s ' "$mark"
  fi
  ui_paint_role painted "$2" "$mark"
  # One cell left and a mark to put in it: the mark wins, exactly as it does at
  # the region's floor. Below one there is nothing honest left to draw.
  if [ "$budget" -lt 1 ]; then printf '%s\n' "$painted" >&2; return 0; fi
  ui_fold "$budget" "$3"
  printf -v pad '%*s' "$gut" ''
  for i in "${!UI_FOLD[@]}"; do
    if [ "$i" -eq 0 ]; then head="$painted"; else head="$pad"; fi
    printf '%s%s\n' "$head" "${UI_FOLD[$i]}" >&2
  done
  return 0
}

ui_say()  { ui__line say    accent "$*"; }
ui_ok()   { ui__line ok     ok     "$*"; }
ui_warn() { ui__line warn   warn   "$*"; }
ui_fail() { ui__line err    err    "$*"; }  # does NOT exit: the caller owns that
ui_info() { ui__line info   muted  "$*"; }
ui_hint() { ui__line hint   muted  "$*"; }

# Stdout, unfolded and unpainted. Reserve it for what a caller captures — a
# path, a JSON document, a rev.
ui_data() { printf '%s\n' "$*"; }

# ── the live region ──────────────────────────────────────────────────────────
# A block of lines rewritten in place: a job list, a phase list, a counter.
#
# The contract in one sentence: the repaint may only move the cursor up by the
# number of SCREEN lines it used, so every row is folded to fit and printed rows
# equal screen lines by construction. Everything else here follows from that.
#
#   ui_row <state> <name> [detail]   buffer a row; state is one of
#                                    run wait ok warn fail skip
#   ui_paint [frame]                 draw the buffered rows and empty the buffer
#   ui_clear                         drop them undrawn
#   ui_live_close                    close the region, restore the cursor
#
# `ui_paint` keeps the frame counter, so a caller painting at ten frames a
# second never has to know which spinner glyph comes next.
declare -ga UI__ROWS=()
declare -gA UI__SEEN=()   # not-a-terminal path: the last state printed per row
UI__PAINTED=0             # screen lines the last frame used
UI__FRAME=0
UI__LIVE=""
UI__PREV_EXIT=""
UI__PREV_INT=""
UI__PREV_WINCH=""

ui_row() { UI__ROWS+=("$1"$'\t'"$2"$'\t'"${3:-}"); }
ui_clear() { UI__ROWS=(); }

# Open the region. Called for you by the first `ui_paint`; call it yourself only
# to hide the cursor earlier than the first frame.
#
# ⚠️ It CHAINS the caller's traps rather than replacing them. A library that
# takes EXIT eats whatever cleanup the script had, and in this family that is a
# scratch worktree or a temp file. `trap -p` prints a re-installable command;
# stripping it back to the bare string and eval-ing the assignment is what
# unquotes it correctly, embedded quotes included.
ui_live_open() {
  [ -n "$UI__LIVE" ] && return 0
  UI__LIVE=1
  [ -z "$UI_TTY" ] && return 0
  local p
  p="$(trap -p EXIT)";  p="${p% EXIT}";  eval "UI__PREV_EXIT=${p#trap -- }"
  p="$(trap -p INT)";   p="${p% INT}";   eval "UI__PREV_INT=${p#trap -- }"
  p="$(trap -p WINCH)"; p="${p% WINCH}"; eval "UI__PREV_WINCH=${p#trap -- }"
  # bash delivers SIGWINCH between commands, so this fires the moment the frame
  # `sleep` returns — a tenth of a second, which reads as instant.
  trap 'ui_measure; eval "$UI__PREV_WINCH"' WINCH
  # Restore the cursor however we leave, Ctrl-C and a `set -e` abort included: a
  # terminal left with no cursor is the worst thing a spinner can do to you.
  # EXIT and not RETURN, because under `set -e` a losing run terminates the
  # shell outright and a RETURN trap never fires on that path.
  trap 'ui_live_close; eval "$UI__PREV_EXIT"' EXIT
  trap 'ui_live_close; eval "$UI__PREV_INT"' INT
  printf '\033[?25l' >&2
  return 0
}

# Close it: restore the cursor, put the traps back, and leave the final frame in
# scrollback. Safe to call twice, and safe from a dying shell.
ui_live_close() {
  [ -z "$UI__LIVE" ] && return 0
  UI__LIVE=""
  if [ -n "$UI_TTY" ]; then
    printf '\033[?25h' >&2
    # Expanding NOW is the point, not an oversight: we are re-installing the
    # command the caller had before we took the signal, which we captured as a
    # string. (SC2064 is right about the usual case and wrong about this one.)
    # shellcheck disable=SC2064
    if [ -n "$UI__PREV_EXIT" ]; then trap "$UI__PREV_EXIT" EXIT; else trap - EXIT; fi
    # shellcheck disable=SC2064
    if [ -n "$UI__PREV_INT" ]; then trap "$UI__PREV_INT" INT; else trap - INT; fi
    # shellcheck disable=SC2064
    if [ -n "$UI__PREV_WINCH" ]; then trap "$UI__PREV_WINCH" WINCH; else trap - WINCH; fi
  fi
  UI__PAINTED=0
  return 0
}

# Draw the buffered rows.
#
# On a stream that is not a terminal this degrades to one plain line per STATE
# CHANGE: no cursor escape ever reaches a file, a pipe or a CI log. Motion,
# unlike colour, is NOT gated on NO_COLOR — a spinner on a colourless terminal
# is still the thing you want to see.
ui_paint() {
  local frame="${1:-}"
  if [ -n "$frame" ]; then UI__FRAME="$frame"; else UI__FRAME=$(( UI__FRAME + 1 )); fi
  ui_live_open
  if [ -z "$UI_TTY" ]; then ui__paint_plain; UI__ROWS=(); return 0; fi

  local -a lines=()
  ui__layout lines

  # A resize reflows whatever is already on screen in ways that cannot be
  # modelled, so don't try: drop to column 0, wipe below, and paint fresh. `[J`
  # clears DOWNWARD, so the pre-resize block stays where it is — one stale copy
  # on screen and nothing after it corrupt. Wiping upward takes real scrollback.
  if [ "$UI_RESIZED" -eq 1 ]; then
    printf '\r\033[J' >&2; UI__PAINTED=0; UI_RESIZED=0
  elif [ "$UI__PAINTED" -gt 0 ]; then
    printf '\033[%dA' "$UI__PAINTED" >&2
  fi
  local l
  for l in "${lines[@]}"; do printf '\033[2K%s\n' "$l" >&2; done
  UI__PAINTED="${#lines[@]}"
  # Wipe whatever a taller previous frame left below: a list shrinks when a
  # snapshot lands late, and a narrower window can drop a row.
  printf '\033[J' >&2
  UI__ROWS=()
  return 0
}

ui__paint_plain() {
  local row state name detail mark g
  for row in "${UI__ROWS[@]}"; do
    IFS=$'\t' read -r state name detail <<<"$row"
    [ "${UI__SEEN[$name]:-}" = "$state" ] && continue
    UI__SEEN[$name]="$state"
    ui__mark g "$state"
    ui_glyph_bare mark "$g"
    printf '   %s %s (%s)\n' "$mark" "$name" "$detail" >&2
  done
  return 0
}

# ui__mark <var> <state> — the glyph name a row state wears.
# ui__role <var> <state> — the role it wears.
#
# Both write into a variable rather than echoing, and that is not style: a live
# region calls them once per row per frame, and `$( )` there is a fork. `bench`
# measured thirty a second from exactly this shape, inside a loop whose own
# comment said everything in it was a builtin.
ui__mark() {
  case "$2" in
    ok) printf -v "$1" 'ok' ;; warn) printf -v "$1" 'warn' ;;
    fail) printf -v "$1" 'err' ;; skip) printf -v "$1" 'skip' ;;
    *) printf -v "$1" 'bullet' ;;
  esac
}

ui__role() {
  case "$2" in
    ok) printf -v "$1" 'ok' ;; warn) printf -v "$1" 'warn' ;;
    fail) printf -v "$1" 'err' ;; run) printf -v "$1" 'accent' ;;
    *) printf -v "$1" 'muted' ;;
  esac
}

# Budget the window across the two columns and render every row.
#
# Three tiers, widest first, each giving up the least useful thing left:
#
#   table — name column padded, detail aligned beside it
#   list  — name only; the detail goes, and so does the padding
#   bare  — the gutter collapses to a single space
#
# ⚠️ The tier is chosen from the WINDOW, then the column is clamped to the
# CONTENT — never the other way round. Clamping first and testing the clamped
# value asks "is the longest name short?" when it means "is there room?", and
# `bench` shipped that inversion for an afternoon: a CI run of build / test /
# lint dropped its durations on a 200-column terminal.
ui__layout() { # ui__layout <array-var>
  local -n __ui_out="$1"
  local row state name detail widest=0 detailw=0 namew gut detailed=1
  local mark painted pname pdetail g r
  __ui_out=()

  # The detail column is MEASURED, never assumed. Budgeting seven cells because
  # "12m 34s" is the longest duration puts "100m 05s" — eight — into the last
  # column and soft-wraps the row, which is the bug this whole region exists to
  # stop.
  for row in "${UI__ROWS[@]}"; do
    IFS=$'\t' read -r state name detail <<<"$row"
    [ "${#name}" -gt "$widest" ] && widest="${#name}"
    [ "${#detail}" -gt "$detailw" ] && detailw="${#detail}"
  done
  [ "$widest" -gt 34 ] && widest=34   # one verbose name can't push every detail right

  # Decided ONCE, before drawing. A per-row decision would let a narrow row's
  # budget leak into the next row and re-widen it past the edge.
  gut=$(( UI_GUTTER + 3 ))            # the gutter, plus the region's own indent
  namew=$(( UI_AVAIL - gut - detailw - 1 ))
  if [ "$namew" -ge 8 ] && [ "$detailw" -gt 0 ]; then
    [ "$namew" -gt "$widest" ] && namew="$widest"
  else
    # Too narrow to align two columns, or nothing to align. The name is the
    # load-bearing half, so the detail goes, and so does the padding that made
    # it a table.
    detailed=0
    [ "$UI_AVAIL" -lt 12 ] && gut=2   # bare: no indent, one space
    namew=$(( UI_AVAIL - gut ))
    [ "$namew" -gt "$widest" ] && namew="$widest"
    # Under four columns there is no room for a name at all, and the glyph alone
    # is still true. Clamping up to 1 here would overflow the edge — the exact
    # class of bug this painter exists to stop.
    [ "$namew" -lt 1 ] && namew=0
  fi

  for row in "${UI__ROWS[@]}"; do
    IFS=$'\t' read -r state name detail <<<"$row"
    # The two narrow tiers take the mark UNPADDED: their gutter is a single
    # space, and two carried cells of padding is two cells past the edge.
    if [ "$state" = run ]; then
      if [ "$namew" -eq 0 ] || [ "$gut" -eq 2 ]; then ui_spin_bare mark "$UI__FRAME"
      else ui_spin mark "$UI__FRAME"; fi
      ui_paint_role painted accent "$mark"
    else
      ui__mark g "$state"; ui__role r "$state"
      if [ "$namew" -eq 0 ] || [ "$gut" -eq 2 ]; then ui_glyph_bare mark "$g"
      else ui_glyph mark "$g"; fi
      ui_paint_role painted "$r" "$mark"
    fi
    ui_truncate name "$name" "$namew"
    if [ "$namew" -eq 0 ]; then
      __ui_out+=("$painted")
    elif [ "$gut" -eq 2 ]; then
      ui_paint_role pname subject "$name"
      __ui_out+=("$painted $pname")
    elif [ "$detailed" -eq 1 ]; then
      # Colour OUTSIDE the width: an escape counted as width shears the column.
      printf -v name '%-*s' "$namew" "$name"
      ui_paint_role pname subject "$name"
      ui_paint_role pdetail muted "$detail"
      __ui_out+=("   $painted$pname $pdetail")
    else
      ui_paint_role pname subject "$name"
      __ui_out+=("   $painted$pname")
    fi
  done
  return 0
}

# ── tables ───────────────────────────────────────────────────────────────────
# A table whose columns are BUDGETED against the window rather than declared in
# a format string. `%-38s` is a width the terminal never agreed to: printf pads,
# so the row occupies its full declared width whatever the content, and the
# threshold at which it wraps becomes a property of the format string and
# nothing else. Seventy-two of those across three CLIs is the defect this
# library was written for.
#
# This is snug's `Table` in bash, and `TestBashTableMatchesGo` diffs the two
# over the same columns and rows at every width — a fallback that laid a table
# out DIFFERENTLY from the binary would be worse than no fallback, because it
# makes "which machine is this?" a question you have to ask about your own
# output.
#
#   ui_col repo    6 1 subject right      # head, min, weight, role, cut side
#   ui_col branch  8 3 body    right
#   ui_col where  10 2 path    left
#   ui_trow bench worktree-cli-beautify-snug ~/.cache/scruff/workshop/x
#   ui_table_data 3 1                     # → fd 1, indented 3, with a header
#
# The cut side is per column and says what that column gives up first: `right`
# keeps the front of a name, `left` the tail of a path (`…/internal/ui`), and
# `never` belongs to a duration or a count — it is right or it is absent, never
# abbreviated.
#
# Below the sum of the minimums the table stops being a table rather than
# emitting a row it knows will wrap: `ui__table_stack` draws one label/value
# pair per line, which has no alignment left to shear.
declare -ga UI__TC_HEAD=() UI__TC_MIN=() UI__TC_WEIGHT=() UI__TC_ROLE=() UI__TC_CUT=()
declare -ga UI__TROWS=()

# ui_col <head> <min> <weight> <role> <cut> — declare one column's appetite.
#
# Min is the width below which the column stops being worth showing; weight
# shares out whatever is left once every column has its minimum.
#
# Head doubles as the column's LABEL: it is what the stacked fallback prints
# beside each value when the window is too narrow for any table at all. So give
# every column a head even when the table draws no header row, which is the
# family default — our tables are read by shape, and a header row on three rows
# of data is furniture.
ui_col() {
  UI__TC_HEAD+=("$1"); UI__TC_MIN+=("${2:-1}"); UI__TC_WEIGHT+=("${3:-1}")
  UI__TC_ROLE+=("${4:-body}"); UI__TC_CUT+=("${5:-right}")
  return 0
}

# The ASCII unit separator brackets the role in a cell that overrides its
# column's — a cell is TEXT here and there is no type to put a role in. Git
# refuses a control character in a ref name and no path, count or duration the
# family draws has ever held one; the split fires only on a leading mark, a role
# this file knows, and a closing mark, so content that somehow contains one is
# still drawn as itself.
UI__CELL_MARK=$'\037'

# ui_cell <var> <role> <text> — a cell that carries its own role.
#
# Most tables want a role per COLUMN and say so once in `ui_col`; that is what to
# reach for. This is for the column whose meaning changes per ROW — a dirty count
# that is amber only when it is not zero, a `↑` that stays quiet while a repo is
# pushed — where the alternative is what `bench` did before it had one: build the
# row with the escapes already in it, and discover the padding now counts them.
ui_cell() {
  printf -v "$1" '%s%s%s%s' "$UI__CELL_MARK" "$2" "$UI__CELL_MARK" "$3"
  return 0
}

# ui__cell_split <role-var> <text-var> <cell> <default-role>
ui__cell_split() {
  local c="$3" r t
  case "$c" in "$UI__CELL_MARK"*"$UI__CELL_MARK"*) ;; *)
    printf -v "$1" '%s' "$4"; printf -v "$2" '%s' "$c"; return 0 ;;
  esac
  t="${c#"$UI__CELL_MARK"}"
  r="${t%%"$UI__CELL_MARK"*}"
  case "$r" in
    body | accent | ok | warn | err | muted | subject | path | field) ;;
    *) printf -v "$1" '%s' "$4"; printf -v "$2" '%s' "$c"; return 0 ;;
  esac
  printf -v "$1" '%s' "$r"
  printf -v "$2" '%s' "${t#*"$UI__CELL_MARK"}"
  return 0
}

# ui_trow <cell> [cell…] — buffer one row.
#
# A cell carries no escapes: colour goes on afterwards, around a finished field,
# or it is counted as width and shears the column. A cell that needs a colour of
# its own says so with `ui_cell`, which is a role and not an escape.
ui_trow() {
  local IFS=$'\t'
  UI__TROWS+=("$*")
  return 0
}

ui_table_clear() {
  UI__TC_HEAD=(); UI__TC_MIN=(); UI__TC_WEIGHT=(); UI__TC_ROLE=(); UI__TC_CUT=()
  UI__TROWS=()
  return 0
}

# ui__split <array-var> <line> — <line> split on TAB, EMPTY FIELDS KEPT.
#
# Not `IFS=$'\t' read -ra`: tab is IFS whitespace whatever IFS is set to, so
# `read` collapses a run of them and every cell after an empty one shifts a
# column left. The live region lives with that — its records never carry an
# empty field between two full ones — but a table's do routinely (a `dirty`
# column is empty on a clean repo), so it cannot.
ui__split() {
  local -n __ui_sp="$1"
  local s="$2"$'\t' f
  __ui_sp=()
  while [ -n "$s" ]; do
    f="${s%%$'\t'*}"
    __ui_sp+=("$f")
    s="${s#*$'\t'}"
  done
  return 0
}

# ui__table_budget <widths-var> <natural-var> <avail> — hand out <avail> cells.
# Returns 1 when even the minimums do not fit, which is the caller's signal to
# stop drawing a table at all.
ui__table_budget() {
  local -n __ui_tw="$1" __ui_tnat="$2"
  local avail="$3" i n sum=0 floor=0 m surplus total weight give moved
  for n in "${__ui_tnat[@]}"; do sum=$(( sum + n )); done
  if [ "$sum" -le "$avail" ]; then __ui_tw=( "${__ui_tnat[@]}" ); return 0; fi

  __ui_tw=()
  for i in "${!__ui_tnat[@]}"; do
    m="${UI__TC_MIN[$i]}"
    # Never reserve more than the column will ever use.
    [ "$m" -gt "${__ui_tnat[$i]}" ] && m="${__ui_tnat[$i]}"
    # A duration is never abbreviated; it is dropped whole. Reserving its
    # natural width is what makes "dropped" the only other outcome.
    [ "${UI__TC_CUT[$i]}" = never ] && m="${__ui_tnat[$i]}"
    __ui_tw+=("$m"); floor=$(( floor + m ))
  done
  [ "$floor" -gt "$avail" ] && return 1

  # Share the surplus by weight, but never past what a column can use. Repeat
  # until nothing moves: a column hitting its natural width releases its share
  # back to the others, which is the difference between one wide column hogging
  # the window and every column being readable.
  surplus=$(( avail - floor ))
  while [ "$surplus" -gt 0 ]; do
    total=0
    for i in "${!__ui_tw[@]}"; do
      [ "${__ui_tw[$i]}" -ge "${__ui_tnat[$i]}" ] && continue
      weight="${UI__TC_WEIGHT[$i]}"; [ "$weight" -lt 1 ] && weight=1
      total=$(( total + weight ))
    done
    if [ "$total" -eq 0 ]; then break; fi
    moved=0
    for i in "${!__ui_tw[@]}"; do
      [ "${__ui_tw[$i]}" -ge "${__ui_tnat[$i]}" ] && continue
      weight="${UI__TC_WEIGHT[$i]}"; [ "$weight" -lt 1 ] && weight=1
      give=$(( surplus * weight / total )); [ "$give" -lt 1 ] && give=1
      [ $(( __ui_tw[i] + give )) -gt "${__ui_tnat[$i]}" ] && give=$(( __ui_tnat[i] - __ui_tw[i] ))
      [ "$give" -gt $(( surplus - moved )) ] && give=$(( surplus - moved ))
      __ui_tw[i]=$(( __ui_tw[i] + give )); moved=$(( moved + give ))
      if [ "$moved" -eq "$surplus" ]; then break; fi
    done
    if [ "$moved" -eq 0 ]; then break; fi
    surplus=$(( surplus - moved ))
  done
  return 0
}

# ui__table_cut <var> <text> <width> <side> — one cell, cut the way its column
# said it gives up cells.
ui__table_cut() {
  case "$4" in
    left)  ui_truncate_left "$1" "$2" "$3" ;;
    never)
      # It is right or it is absent. A duration cut to `12m…` is a lie with a
      # mark on it.
      if [ "${#2}" -gt "$3" ]; then printf -v "$1" '%s' ''
      else printf -v "$1" '%s' "$2"; fi
      ;;
    *)     ui_truncate "$1" "$2" "$3" ;;
  esac
  return 0
}

# ui__table_stack <out-array> <avail> <prefix> — the fallback below the sum of
# the minimums: one label/value pair per line.
#
# Bound by the same rule as everything else — nothing wider than the window —
# which is why the LABEL is truncated too. A fallback that overflows is not a
# fallback.
ui__table_stack() {
  local -n __ui_st="$1"
  local avail="$2" pfx="$3"
  local -a cells=()
  local n i j w v role label lbl painted pad
  __ui_st=()
  for n in "${!UI__TROWS[@]}"; do
    [ "$n" -gt 0 ] && __ui_st+=("")
    ui__split cells "${UI__TROWS[$n]}"
    for i in "${!UI__TC_HEAD[@]}"; do
      [ "$i" -lt "${#cells[@]}" ] || continue
      ui__cell_split role v "${cells[$i]}" "${UI__TC_ROLE[$i]}"
      [ -n "$v" ] || continue
      ui_truncate label "${UI__TC_HEAD[$i]} " "$avail" ''
      ui_paint_role lbl field "$label" "$pfx"
      if [ $(( avail - ${#label} )) -lt 1 ]; then
        # No room for a value beside its label. Give the label its own line and
        # let the value fold under it.
        __ui_st+=("$lbl")
        w="$avail"; [ "$w" -lt 1 ] && w=1
        ui_fold "$w" "$v"
        for j in "${!UI_FOLD[@]}"; do
          ui_paint_role painted "$role" "${UI_FOLD[$j]}" "$pfx"
          __ui_st+=("$painted")
        done
        continue
      fi
      # A column that declared which END matters keeps that promise here too:
      # folding a path breaks it mid-component (`…nebelun` / `g/…`), which reads
      # as a directory that does not exist. One left-cut line says more than
      # three ragged ones.
      if [ "${UI__TC_CUT[$i]}" = left ]; then
        ui_truncate_left v "$v" $(( avail - ${#label} ))
        ui_paint_role painted "$role" "$v" "$pfx"
        __ui_st+=("$lbl$painted")
        continue
      fi
      printf -v pad '%*s' "${#label}" ''
      ui_fold $(( avail - ${#label} )) "$v"
      for j in "${!UI_FOLD[@]}"; do
        ui_paint_role painted "$role" "${UI_FOLD[$j]}" "$pfx"
        if [ "$j" -eq 0 ]; then __ui_st+=("$lbl$painted")
        else __ui_st+=("$pad$painted"); fi
      done
    done
  done
  return 0
}

# ui__table_render <out-array> <avail> <prefix> <indent> <header> — the finished
# lines. Everything above is arithmetic; this is the only part that paints.
ui__table_render() {
  local -n __ui_lines="$1"
  local avail="$2" pfx="$3" indent="$4" header="$5"
  local -a natural=() widths=() cells=() all=()
  local i n cols row v role line pad ifs
  __ui_lines=()
  cols="${#UI__TC_HEAD[@]}"
  if [ "$cols" -eq 0 ] || [ "${#UI__TROWS[@]}" -eq 0 ]; then return 0; fi

  # The natural width is measured from the content AND the head, never assumed.
  # Budgeting seven cells because "12m 34s" is the longest duration is how
  # "100m 05s" — eight — ended up in the next column.
  for (( i = 0; i < cols; i++ )); do natural+=("${#UI__TC_HEAD[$i]}"); done
  for row in "${UI__TROWS[@]}"; do
    ui__split cells "$row"
    for (( i = 0; i < cols; i++ )); do
      [ "$i" -lt "${#cells[@]}" ] || continue
      # The tag is not content and is never drawn, so it is never measured
      # either: budgeting it reserves cells for something nobody can see.
      ui__cell_split role v "${cells[$i]}" body
      [ "${#v}" -gt "${natural[$i]}" ] && natural[i]="${#v}"
    done
  done

  # One space between columns; two reads as a gutter rather than a gap.
  if ! ui__table_budget widths natural $(( avail - indent - (cols - 1) )); then
    ui__table_stack __ui_lines "$avail" "$pfx"
    return 0
  fi

  if [ "$header" = 1 ]; then
    ifs="$IFS"; IFS=$'\t'; all+=("${UI__TC_HEAD[*]}"); IFS="$ifs"
  fi
  all+=("${UI__TROWS[@]}")
  printf -v pad '%*s' "$indent" ''
  for n in "${!all[@]}"; do
    ui__split cells "${all[$n]}"
    line="$pad"
    for (( i = 0; i < cols; i++ )); do
      v=""; role="${UI__TC_ROLE[$i]}"
      [ "$i" -lt "${#cells[@]}" ] && ui__cell_split role v "${cells[$i]}" "$role"
      ui__table_cut v "$v" "${widths[$i]}" "${UI__TC_CUT[$i]}"
      # Never pad the last column: trailing spaces are what wrap a row that
      # just fit.
      [ "$i" -lt $(( cols - 1 )) ] && ui_pad v "$v" "${widths[$i]}"
      # A head is a head whatever its column paints.
      if [ "$header" = 1 ] && [ "$n" -eq 0 ]; then role=field; fi
      # Colour OUTSIDE the width: an escape counted as width shears the column.
      ui_paint_role v "$role" "$v" "$pfx"
      if [ "$i" -eq 0 ]; then line+="$v"; else line+=" $v"; fi
    done
    __ui_lines+=("${line%"${line##*[! ]}"}")
  done
  return 0
}

# ui_table [indent] [header] — the table on STDERR, part of what the tool is
# SAYING. It scrolls above an open live region like any other line.
ui_table() {
  local -a __ui_t=()
  local l
  ui__table_render __ui_t "$UI_AVAIL" UI_ "${1:-$UI_GUTTER}" "${2:-0}"
  for l in ${__ui_t[@]+"${__ui_t[@]}"}; do printf '%s\n' "$l" >&2; done
  ui_table_clear
  return 0
}

# ui_table_data [indent] [header] — the table on STDOUT, because it is the
# REPORT: the thing the user ran the command for rather than the tool talking
# about it. `bench status | less` carries it whole, and the narration around it
# stays on fd 2.
#
# Measured, gated and painted for fd 1 — which is the whole reason `UI_OUT_*`
# exists. Asking fd 2 about a report costs it in both directions: a TTY stdout
# beside a redirected stderr would draw plain, and a piped stdout beside a live
# stderr would draw escapes into the pipe.
#
# ⚠️ Close any live region first. Like `ui_data`, this does not cooperate with
# one, and when both streams are the same terminal it is destructive rather than
# merely uncooperative: the region's next repaint walks up the lines IT wrote
# and clears downward from a cursor this has since moved, erasing the report the
# user actually ran the command for.
ui_table_data() {
  local -a __ui_t=()
  local l
  ui__table_render __ui_t "$UI_OUT_AVAIL" UI_OUT_ "${1:-$UI_GUTTER}" "${2:-0}"
  for l in ${__ui_t[@]+"${__ui_t[@]}"}; do printf '%s\n' "$l"; done
  ui_table_clear
  return 0
}

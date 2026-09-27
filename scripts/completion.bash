# Bash completion for ctxpack.
#
# Source it from ~/.bashrc:
#   source /path/to/ctxpack/scripts/completion.bash
#
# The tables below are the executable form of the "FLAGS (...)" sections in
# `ctxpack --help`. When they drift, `make helpcheck`-style guards do not exist
# yet, so regenerate them by hand from printHelp.

_ctxpack_commands="pack map diff tokens models mcp doctor version help"

# Per-command flags: shared walk flags first, then the command's own section.
_ctxpack_walk="--include --exclude --max-size --no-gitignore --hidden --depth"

_ctxpack_flags() {
  local cmd="$1"
  case "$cmd" in
    pack)   echo "$_ctxpack_walk --format --budget --model --dry-run --output -o -q --quiet" ;;
    diff)   echo "$_ctxpack_walk --format --ref --list --budget --model --dry-run --output -o -q --quiet" ;;
    map)    echo "$_ctxpack_walk --format --json --sort --top --csv --output -o" ;;
    tokens) echo "$_ctxpack_walk --format --json --csv --sort --top --model --output -o" ;;
    models) echo "--format --json --csv --sort --top --vendor --output -o" ;;
    doctor) echo "--format --json --top --output -o" ;;
    version) echo "--json --short" ;;
    mcp|help) echo "" ;;
  esac
}

_ctxpack() {
  local cur prev words cword
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  words=("${COMP_WORDS[@]}")
  cword="$COMP_CWORD"

  if [ "$cword" -eq 1 ]; then
    # First word: a command name, a help flag, or --version/-v.
    COMPREPLY=($(compgen -W "$_ctxpack_commands --help -h --version -v" -- "$cur"))
    return 0
  fi

  local cmd="${COMP_WORDS[1]}"
  case "$cmd" in
    pack|diff|map|tokens|models|doctor|version|mcp|help)
      COMPREPLY=($(compgen -W "$(_ctxpack_flags "$cmd")" -- "$cur"))
      ;;
    *)
      COMPREPLY=($(compgen -W "$_ctxpack_commands" -- "$cur"))
      ;;
  esac
  return 0
}

complete -o default -F _ctxpack ctxpack
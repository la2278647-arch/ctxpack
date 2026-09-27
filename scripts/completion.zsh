# Zsh completion for ctxpack.
#
# Source it from ~/.zshrc:
#   source /path/to/ctxpack/scripts/completion.zsh
#
# Uses compdef; requires the completion system to be enabled:
#   autoload -U compinit && compinit

# The tables below mirror scripts/completion.bash and the "FLAGS (...)"
# sections in `ctxpack --help`.

_ctxpack_commands=(pack map diff tokens models mcp doctor version help)
_ctxpack_walk=(--include --exclude --max-size --no-gitignore --hidden --depth)

_ctxpack_per_cmd() {
  local cmd=$1
  case "$cmd" in
    pack)   print -l $_ctxpack_walk --format --budget --model --dry-run --output -o -q ;;
    diff)   print -l $_ctxpack_walk --format --ref --list --budget --model --dry-run --output -o -q ;;
    map)    print -l $_ctxpack_walk --format --json --sort --top --csv --output -o ;;
    tokens) print -l $_ctxpack_walk --format --json --csv --sort --top --model --output -o ;;
    models) print -l --format --json --csv --sort --top --vendor --output -o ;;
    doctor) print -l --format --json --top --output -o ;;
    version) print -l --json ;;
    mcp|help) ;;
  esac
}

_ctxpack() {
  local -a flags
  if (( CURRENT == 2 )); then
    _values 'ctxpack command' ${_ctxpack_commands[@]} --help -h --version -v
    return
  fi
  flags=(${(f)"$(_ctxpack_per_cmd ${words[2]})"})
  _values 'ctxpack flag' ${flags[@]}
}

compdef _ctxpack ctxpack
# Fish completion for ctxpack.
#
# Put it in ~/.config/fish/completions/ctxpack.fish (fish sources files there
# automatically), or `source` it manually.
#
# The flag tables mirror scripts/completion.bash / completion.zsh and the
# "FLAGS (...)" sections in `ctxpack --help`.

# Commands.
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'pack' -d 'Pack a repository into one context bundle'
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'map' -d 'Token-aware tree outline'
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'diff' -d 'Pack only the files changed in git'
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'tokens' -d 'Total token estimate + per-model fit'
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'models' -d 'List known models and context windows'
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'mcp' -d 'Run as an MCP server on stdio'
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'doctor' -d 'Print environment diagnostics'
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'version' -d 'Print the build identity'
complete -c ctxpack -f -n '__fish_use_subcommand' -a 'help' -d 'Show this help'

# Walk flags, shared by pack / diff / map / tokens.
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff map tokens' -l include   -r -d 'Only include paths matching GLOB (repeatable)'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff map tokens' -l exclude   -r -d 'Exclude paths matching GLOB (repeatable)'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff map tokens' -l max-size  -r -d 'Read no more than N bytes of a file'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff map tokens' -l no-gitignore -d 'Ignore .gitignore files'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff map tokens' -l hidden     -d 'Include dotfiles and dot-directories'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff map tokens' -l depth      -r -d 'Limit traversal to N levels below root'

# pack / diff.
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff' -l format -x -a 'xml markdown json text' -d 'Output format'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff' -l budget -r -d 'Cap output to ~N tokens (priority-selects files)'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff' -l model  -r -d 'Annotate fit for a model'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff' -l output -r -d 'Write to FILE instead of stdout'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff' -s o      -r -d 'Shorthand for --output'
complete -c ctxpack -n '__fish_seen_subcommand_from pack diff' -s q      -d 'Suppress stderr status messages'
complete -c ctxpack -n '__fish_seen_subcommand_from pack'    -l dry-run -d 'Show what would be packed without writing'
complete -c ctxpack -n '__fish_seen_subcommand_from diff'    -l ref     -r -d 'Base git ref (e.g. HEAD~1, main)'
complete -c ctxpack -n '__fish_seen_subcommand_from diff'    -l list    -d 'List the paths the pack will name'
complete -c ctxpack -n '__fish_seen_subcommand_from diff'    -l dry-run -d 'Show changed files without writing'

# map.
complete -c ctxpack -n '__fish_seen_subcommand_from map' -l format -x -a 'text json csv' -d 'Output format'
complete -c ctxpack -n '__fish_seen_subcommand_from map' -l json  -d 'Emit JSON instead of the text output'
complete -c ctxpack -n '__fish_seen_subcommand_from map' -l csv   -d 'Flat CSV list of all files'
complete -c ctxpack -n '__fish_seen_subcommand_from map' -l sort  -x -a 'name tokens bytes' -d 'Sort children by'
complete -c ctxpack -n '__fish_seen_subcommand_from map' -l top   -r -d 'Flat list of the N largest files'
complete -c ctxpack -n '__fish_seen_subcommand_from map' -l output -r -d 'Write to FILE instead of stdout'
complete -c ctxpack -n '__fish_seen_subcommand_from map' -s o      -r -d 'Shorthand for --output'

# tokens.
complete -c ctxpack -n '__fish_seen_subcommand_from tokens' -l format -x -a 'text json csv' -d 'Output format'
complete -c ctxpack -n '__fish_seen_subcommand_from tokens' -l json  -d 'Emit JSON instead of the text output'
complete -c ctxpack -n '__fish_seen_subcommand_from tokens' -l csv   -d 'Emit the per-model fit as CSV'
complete -c ctxpack -n '__fish_seen_subcommand_from tokens' -l sort  -x -a 'name pct window' -d 'Sort the fit table by'
complete -c ctxpack -n '__fish_seen_subcommand_from tokens' -l top   -r -d 'Show only the N largest models by window'
complete -c ctxpack -n '__fish_seen_subcommand_from tokens' -l model -r -d 'Show fit for one model instead of all'
complete -c ctxpack -n '__fish_seen_subcommand_from tokens' -l output -r -d 'Write to FILE instead of stdout'
complete -c ctxpack -n '__fish_seen_subcommand_from tokens' -s o      -r -d 'Shorthand for --output'

# models.
complete -c ctxpack -n '__fish_seen_subcommand_from models' -l format -x -a 'text json csv' -d 'Output format'
complete -c ctxpack -n '__fish_seen_subcommand_from models' -l json   -d 'Emit JSON instead of the text output'
complete -c ctxpack -n '__fish_seen_subcommand_from models' -l csv    -d 'Emit CSV (name,context_window,vendor)'
complete -c ctxpack -n '__fish_seen_subcommand_from models' -l sort   -x -a 'name window vendor' -d 'Sort by'
complete -c ctxpack -n '__fish_seen_subcommand_from models' -l top    -r -d 'Show only the N largest models by window'
complete -c ctxpack -n '__fish_seen_subcommand_from models' -l vendor -r -d 'Show only models from this vendor'
complete -c ctxpack -n '__fish_seen_subcommand_from models' -l output -r -d 'Write to FILE instead of stdout'
complete -c ctxpack -n '__fish_seen_subcommand_from models' -s o      -r -d 'Shorthand for --output'

# doctor.
complete -c ctxpack -n '__fish_seen_subcommand_from doctor' -l format -x -a 'text json' -d 'Output format'
complete -c ctxpack -n '__fish_seen_subcommand_from doctor' -l json   -d 'Emit JSON instead of the text output'
complete -c ctxpack -n '__fish_seen_subcommand_from doctor' -l top    -r -d 'Show only the top N vendors'
complete -c ctxpack -n '__fish_seen_subcommand_from doctor' -l output -r -d 'Write to FILE instead of stdout'
complete -c ctxpack -n '__fish_seen_subcommand_from doctor' -s o      -r -d 'Shorthand for --output'

# version.
complete -c ctxpack -n '__fish_seen_subcommand_from version' -l json -d 'Emit the build identity as one JSON object'
# The console's default .zshrc. Copied into the home volume on first start and
# never again, so this copy is yours to change.

# --- history -----------------------------------------------------------------
# The home directory is a volume, so history outlives the pod. Shared and
# appended as it is written because a console usually has several tmux windows
# open at once, and a command typed in one should be reachable from the others.
HISTFILE="$HOME/.zsh_history"
HISTSIZE=1000000
SAVEHIST=1000000
setopt INC_APPEND_HISTORY SHARE_HISTORY HIST_IGNORE_DUPS HIST_REDUCE_BLANKS EXTENDED_HISTORY

# --- completion --------------------------------------------------------------
autoload -Uz compinit
compinit -d "$HOME/.zcompdump"
zstyle ':completion:*' menu select

# Generated per shell rather than cached, so they always describe the binaries
# that are actually installed.
if command -v kubectl >/dev/null 2>&1; then
  source <(kubectl completion zsh)
  alias k=kubectl
  compdef k=kubectl
fi
if command -v helm >/dev/null 2>&1; then
  source <(helm completion zsh)
fi

# --- editing -----------------------------------------------------------------
bindkey -e
export EDITOR=vim PAGER=less LESS=-R

# --- prompt ------------------------------------------------------------------
# user@console, the working directory, and the kube context. In the pod there is
# usually no kubeconfig and kubectl uses the ServiceAccount, which has no context
# name, so that case is shown as "in-cluster" rather than as nothing.
setopt PROMPT_SUBST
_console_kube_context() {
  local ctx
  ctx=$(kubectl config current-context 2>/dev/null) || ctx=in-cluster
  print -r -- "${ctx:-in-cluster}"
}
PROMPT='%F{green}%n@console%f %F{blue}%~%f %F{yellow}($(_console_kube_context))%f
%(?.%F{white}.%F{red})%#%f '

# --- plugins -----------------------------------------------------------------
# Syntax highlighting last: it wraps the widgets defined before it.
source /usr/share/zsh/plugins/zsh-autosuggestions/zsh-autosuggestions.zsh
source /usr/share/zsh/plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh

# bash completion for cozy-stack                           -*- shell-script -*-

__cozy-stack_debug()
{
    if [[ -n ${BASH_COMP_DEBUG_FILE} ]]; then
        echo "$*" >> "${BASH_COMP_DEBUG_FILE}"
    fi
}

# Homebrew on Macs have version 1.3 of bash-completion which doesn't include
# _init_completion. This is a very minimal version of that function.
__cozy-stack_init_completion()
{
    COMPREPLY=()
    _get_comp_words_by_ref "$@" cur prev words cword
}

__cozy-stack_index_of_word()
{
    local w word=$1
    shift
    index=0
    for w in "$@"; do
        [[ $w = "$word" ]] && return
        index=$((index+1))
    done
    index=-1
}

__cozy-stack_contains_word()
{
    local w word=$1; shift
    for w in "$@"; do
        [[ $w = "$word" ]] && return
    done
    return 1
}

__cozy-stack_handle_reply()
{
    __cozy-stack_debug "${FUNCNAME[0]}"
    case $cur in
        -*)
            if [[ $(type -t compopt) = "builtin" ]]; then
                compopt -o nospace
            fi
            local allflags
            if [ ${#must_have_one_flag[@]} -ne 0 ]; then
                allflags=("${must_have_one_flag[@]}")
            else
                allflags=("${flags[*]} ${two_word_flags[*]}")
            fi
            COMPREPLY=( $(compgen -W "${allflags[*]}" -- "$cur") )
            if [[ $(type -t compopt) = "builtin" ]]; then
                [[ "${COMPREPLY[0]}" == *= ]] || compopt +o nospace
            fi

            # complete after --flag=abc
            if [[ $cur == *=* ]]; then
                if [[ $(type -t compopt) = "builtin" ]]; then
                    compopt +o nospace
                fi

                local index flag
                flag="${cur%=*}"
                __cozy-stack_index_of_word "${flag}" "${flags_with_completion[@]}"
                COMPREPLY=()
                if [[ ${index} -ge 0 ]]; then
                    PREFIX=""
                    cur="${cur#*=}"
                    ${flags_completion[${index}]}
                    if [ -n "${ZSH_VERSION}" ]; then
                        # zsh completion needs --flag= prefix
                        eval "COMPREPLY=( \"\${COMPREPLY[@]/#/${flag}=}\" )"
                    fi
                fi
            fi
            return 0;
            ;;
    esac

    # check if we are handling a flag with special work handling
    local index
    __cozy-stack_index_of_word "${prev}" "${flags_with_completion[@]}"
    if [[ ${index} -ge 0 ]]; then
        ${flags_completion[${index}]}
        return
    fi

    # we are parsing a flag and don't have a special handler, no completion
    if [[ ${cur} != "${words[cword]}" ]]; then
        return
    fi

    local completions
    completions=("${commands[@]}")
    if [[ ${#must_have_one_noun[@]} -ne 0 ]]; then
        completions=("${must_have_one_noun[@]}")
    fi
    if [[ ${#must_have_one_flag[@]} -ne 0 ]]; then
        completions+=("${must_have_one_flag[@]}")
    fi
    COMPREPLY=( $(compgen -W "${completions[*]}" -- "$cur") )

    if [[ ${#COMPREPLY[@]} -eq 0 && ${#noun_aliases[@]} -gt 0 && ${#must_have_one_noun[@]} -ne 0 ]]; then
        COMPREPLY=( $(compgen -W "${noun_aliases[*]}" -- "$cur") )
    fi

    if [[ ${#COMPREPLY[@]} -eq 0 ]]; then
		if declare -F __cozy-stack_custom_func >/dev/null; then
			# try command name qualified custom func
			__cozy-stack_custom_func
		else
			# otherwise fall back to unqualified for compatibility
			declare -F __custom_func >/dev/null && __custom_func
		fi
    fi

    # available in bash-completion >= 2, not always present on macOS
    if declare -F __ltrim_colon_completions >/dev/null; then
        __ltrim_colon_completions "$cur"
    fi

    # If there is only 1 completion and it is a flag with an = it will be completed
    # but we don't want a space after the =
    if [[ "${#COMPREPLY[@]}" -eq "1" ]] && [[ $(type -t compopt) = "builtin" ]] && [[ "${COMPREPLY[0]}" == --*= ]]; then
       compopt -o nospace
    fi
}

# The arguments should be in the form "ext1|ext2|extn"
__cozy-stack_handle_filename_extension_flag()
{
    local ext="$1"
    _filedir "@(${ext})"
}

__cozy-stack_handle_subdirs_in_dir_flag()
{
    local dir="$1"
    pushd "${dir}" >/dev/null 2>&1 && _filedir -d && popd >/dev/null 2>&1
}

__cozy-stack_handle_flag()
{
    __cozy-stack_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    # if a command required a flag, and we found it, unset must_have_one_flag()
    local flagname=${words[c]}
    local flagvalue
    # if the word contained an =
    if [[ ${words[c]} == *"="* ]]; then
        flagvalue=${flagname#*=} # take in as flagvalue after the =
        flagname=${flagname%=*} # strip everything after the =
        flagname="${flagname}=" # but put the = back
    fi
    __cozy-stack_debug "${FUNCNAME[0]}: looking for ${flagname}"
    if __cozy-stack_contains_word "${flagname}" "${must_have_one_flag[@]}"; then
        must_have_one_flag=()
    fi

    # if you set a flag which only applies to this command, don't show subcommands
    if __cozy-stack_contains_word "${flagname}" "${local_nonpersistent_flags[@]}"; then
      commands=()
    fi

    # keep flag value with flagname as flaghash
    # flaghash variable is an associative array which is only supported in bash > 3.
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        if [ -n "${flagvalue}" ] ; then
            flaghash[${flagname}]=${flagvalue}
        elif [ -n "${words[ $((c+1)) ]}" ] ; then
            flaghash[${flagname}]=${words[ $((c+1)) ]}
        else
            flaghash[${flagname}]="true" # pad "true" for bool flag
        fi
    fi

    # skip the argument to a two word flag
    if [[ ${words[c]} != *"="* ]] && __cozy-stack_contains_word "${words[c]}" "${two_word_flags[@]}"; then
			  __cozy-stack_debug "${FUNCNAME[0]}: found a flag ${words[c]}, skip the next argument"
        c=$((c+1))
        # if we are looking for a flags value, don't show commands
        if [[ $c -eq $cword ]]; then
            commands=()
        fi
    fi

    c=$((c+1))

}

__cozy-stack_handle_noun()
{
    __cozy-stack_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    if __cozy-stack_contains_word "${words[c]}" "${must_have_one_noun[@]}"; then
        must_have_one_noun=()
    elif __cozy-stack_contains_word "${words[c]}" "${noun_aliases[@]}"; then
        must_have_one_noun=()
    fi

    nouns+=("${words[c]}")
    c=$((c+1))
}

__cozy-stack_handle_command()
{
    __cozy-stack_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    local next_command
    if [[ -n ${last_command} ]]; then
        next_command="_${last_command}_${words[c]//:/__}"
    else
        if [[ $c -eq 0 ]]; then
            next_command="_cozy-stack_root_command"
        else
            next_command="_${words[c]//:/__}"
        fi
    fi
    c=$((c+1))
    __cozy-stack_debug "${FUNCNAME[0]}: looking for ${next_command}"
    declare -F "$next_command" >/dev/null && $next_command
}

__cozy-stack_handle_word()
{
    if [[ $c -ge $cword ]]; then
        __cozy-stack_handle_reply
        return
    fi
    __cozy-stack_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"
    if [[ "${words[c]}" == -* ]]; then
        __cozy-stack_handle_flag
    elif __cozy-stack_contains_word "${words[c]}" "${commands[@]}"; then
        __cozy-stack_handle_command
    elif [[ $c -eq 0 ]]; then
        __cozy-stack_handle_command
    elif __cozy-stack_contains_word "${words[c]}" "${command_aliases[@]}"; then
        # aliashash variable is an associative array which is only supported in bash > 3.
        if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
            words[c]=${aliashash[${words[c]}]}
            __cozy-stack_handle_command
        else
            __cozy-stack_handle_noun
        fi
    else
        __cozy-stack_handle_noun
    fi
    __cozy-stack_handle_word
}

_cozy-stack_apps_install()
{
    last_command="cozy-stack_apps_install"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--ask-permissions")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_apps_ls()
{
    last_command="cozy-stack_apps_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_apps_show()
{
    last_command="cozy-stack_apps_show"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_apps_uninstall()
{
    last_command="cozy-stack_apps_uninstall"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_apps_update()
{
    last_command="cozy-stack_apps_update"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--safe")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_apps()
{
    last_command="cozy-stack_apps"

    command_aliases=()

    commands=()
    commands+=("install")
    commands+=("ls")
    commands+=("show")
    commands+=("uninstall")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("rm")
        aliashash["rm"]="uninstall"
    fi
    commands+=("update")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("upgrade")
        aliashash["upgrade"]="update"
    fi

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--all-domains")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_assets_add()
{
    last_command="cozy-stack_assets_add"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--context=")
    two_word_flags+=("--context")
    local_nonpersistent_flags+=("--context=")
    flags+=("--name=")
    two_word_flags+=("--name")
    local_nonpersistent_flags+=("--name=")
    flags+=("--shasum=")
    two_word_flags+=("--shasum")
    local_nonpersistent_flags+=("--shasum=")
    flags+=("--url=")
    two_word_flags+=("--url")
    local_nonpersistent_flags+=("--url=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_assets_ls()
{
    last_command="cozy-stack_assets_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_assets_rm()
{
    last_command="cozy-stack_assets_rm"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_assets()
{
    last_command="cozy-stack_assets"

    command_aliases=()

    commands=()
    commands+=("add")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("insert")
        aliashash["insert"]="add"
    fi
    commands+=("ls")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("list")
        aliashash["list"]="ls"
    fi
    commands+=("rm")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("remove")
        aliashash["remove"]="rm"
    fi

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_bug()
{
    last_command="cozy-stack_bug"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_completion()
{
    last_command="cozy-stack_completion"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    must_have_one_noun+=("bash")
    noun_aliases=()
}

_cozy-stack_config_decrypt-creds()
{
    last_command="cozy-stack_config_decrypt-creds"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_decrypt-data()
{
    last_command="cozy-stack_config_decrypt-data"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_encrypt-creds()
{
    last_command="cozy-stack_config_encrypt-creds"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_encrypt-data()
{
    last_command="cozy-stack_config_encrypt-data"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_gen-keys()
{
    last_command="cozy-stack_config_gen-keys"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_insert-asset()
{
    last_command="cozy-stack_config_insert-asset"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--context=")
    two_word_flags+=("--context")
    local_nonpersistent_flags+=("--context=")
    flags+=("--name=")
    two_word_flags+=("--name")
    local_nonpersistent_flags+=("--name=")
    flags+=("--shasum=")
    two_word_flags+=("--shasum")
    local_nonpersistent_flags+=("--shasum=")
    flags+=("--url=")
    two_word_flags+=("--url")
    local_nonpersistent_flags+=("--url=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_ls-assets()
{
    last_command="cozy-stack_config_ls-assets"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_ls-contexts()
{
    last_command="cozy-stack_config_ls-contexts"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_passwd()
{
    last_command="cozy-stack_config_passwd"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config_rm-asset()
{
    last_command="cozy-stack_config_rm-asset"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_config()
{
    last_command="cozy-stack_config"

    command_aliases=()

    commands=()
    commands+=("decrypt-creds")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("decrypt-credentials")
        aliashash["decrypt-credentials"]="decrypt-creds"
    fi
    commands+=("decrypt-data")
    commands+=("encrypt-creds")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("encrypt-credentials")
        aliashash["encrypt-credentials"]="encrypt-creds"
    fi
    commands+=("encrypt-data")
    commands+=("gen-keys")
    commands+=("insert-asset")
    commands+=("ls-assets")
    commands+=("ls-contexts")
    commands+=("passwd")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("pass")
        aliashash["pass"]="passwd"
        command_aliases+=("passphrase")
        aliashash["passphrase"]="passwd"
        command_aliases+=("password")
        aliashash["password"]="passwd"
    fi
    commands+=("rm-asset")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_doc_man()
{
    last_command="cozy-stack_doc_man"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_doc_markdown()
{
    last_command="cozy-stack_doc_markdown"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_doc()
{
    last_command="cozy-stack_doc"

    command_aliases=()

    commands=()
    commands+=("man")
    commands+=("markdown")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_features_defaults()
{
    last_command="cozy-stack_features_defaults"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_features_flags()
{
    last_command="cozy-stack_features_flags"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    local_nonpersistent_flags+=("--domain=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_features_ratio()
{
    last_command="cozy-stack_features_ratio"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--context=")
    two_word_flags+=("--context")
    local_nonpersistent_flags+=("--context=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_features_sets()
{
    last_command="cozy-stack_features_sets"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    local_nonpersistent_flags+=("--domain=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_features_show()
{
    last_command="cozy-stack_features_show"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    local_nonpersistent_flags+=("--domain=")
    flags+=("--source")
    local_nonpersistent_flags+=("--source")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_features()
{
    last_command="cozy-stack_features"

    command_aliases=()

    commands=()
    commands+=("defaults")
    commands+=("flags")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("flag")
        aliashash["flag"]="flags"
    fi
    commands+=("ratio")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("context")
        aliashash["context"]="ratio"
    fi
    commands+=("sets")
    commands+=("show")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_files_exec()
{
    last_command="cozy-stack_files_exec"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_files_import()
{
    last_command="cozy-stack_files_import"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--dry-run")
    local_nonpersistent_flags+=("--dry-run")
    flags+=("--from=")
    two_word_flags+=("--from")
    local_nonpersistent_flags+=("--from=")
    flags+=("--match=")
    two_word_flags+=("--match")
    local_nonpersistent_flags+=("--match=")
    flags+=("--to=")
    two_word_flags+=("--to")
    local_nonpersistent_flags+=("--to=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_flag+=("--from=")
    must_have_one_flag+=("--to=")
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_files_usage()
{
    last_command="cozy-stack_files_usage"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_files()
{
    last_command="cozy-stack_files"

    command_aliases=()

    commands=()
    commands+=("exec")
    commands+=("import")
    commands+=("usage")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_contact-emails()
{
    last_command="cozy-stack_fixer_contact-emails"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_content-mismatch()
{
    last_command="cozy-stack_fixer_content-mismatch"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--no-dry-run")
    local_nonpersistent_flags+=("--no-dry-run")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_indexes()
{
    last_command="cozy-stack_fixer_indexes"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_jobs()
{
    last_command="cozy-stack_fixer_jobs"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_md5()
{
    last_command="cozy-stack_fixer_md5"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_mime()
{
    last_command="cozy-stack_fixer_mime"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_orphan-account()
{
    last_command="cozy-stack_fixer_orphan-account"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_redis()
{
    last_command="cozy-stack_fixer_redis"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer_thumbnails()
{
    last_command="cozy-stack_fixer_thumbnails"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--dry-run")
    local_nonpersistent_flags+=("--dry-run")
    flags+=("--with-metadata")
    local_nonpersistent_flags+=("--with-metadata")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_fixer()
{
    last_command="cozy-stack_fixer"

    command_aliases=()

    commands=()
    commands+=("contact-emails")
    commands+=("content-mismatch")
    commands+=("indexes")
    commands+=("jobs")
    commands+=("md5")
    commands+=("mime")
    commands+=("orphan-account")
    commands+=("redis")
    commands+=("thumbnails")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_add()
{
    last_command="cozy-stack_instances_add"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--apps=")
    two_word_flags+=("--apps")
    local_nonpersistent_flags+=("--apps=")
    flags+=("--context-name=")
    two_word_flags+=("--context-name")
    local_nonpersistent_flags+=("--context-name=")
    flags+=("--dev")
    local_nonpersistent_flags+=("--dev")
    flags+=("--disk-quota=")
    two_word_flags+=("--disk-quota")
    local_nonpersistent_flags+=("--disk-quota=")
    flags+=("--domain-aliases=")
    two_word_flags+=("--domain-aliases")
    local_nonpersistent_flags+=("--domain-aliases=")
    flags+=("--email=")
    two_word_flags+=("--email")
    local_nonpersistent_flags+=("--email=")
    flags+=("--locale=")
    two_word_flags+=("--locale")
    local_nonpersistent_flags+=("--locale=")
    flags+=("--passphrase=")
    two_word_flags+=("--passphrase")
    local_nonpersistent_flags+=("--passphrase=")
    flags+=("--public-name=")
    two_word_flags+=("--public-name")
    local_nonpersistent_flags+=("--public-name=")
    flags+=("--settings=")
    two_word_flags+=("--settings")
    local_nonpersistent_flags+=("--settings=")
    flags+=("--swift-layout=")
    two_word_flags+=("--swift-layout")
    local_nonpersistent_flags+=("--swift-layout=")
    flags+=("--tos=")
    two_word_flags+=("--tos")
    local_nonpersistent_flags+=("--tos=")
    flags+=("--tz=")
    two_word_flags+=("--tz")
    local_nonpersistent_flags+=("--tz=")
    flags+=("--uuid=")
    two_word_flags+=("--uuid")
    local_nonpersistent_flags+=("--uuid=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_auth-mode()
{
    last_command="cozy-stack_instances_auth-mode"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_client-oauth()
{
    last_command="cozy-stack_instances_client-oauth"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--allow-login-scope")
    local_nonpersistent_flags+=("--allow-login-scope")
    flags+=("--json")
    local_nonpersistent_flags+=("--json")
    flags+=("--onboarding-app=")
    two_word_flags+=("--onboarding-app")
    local_nonpersistent_flags+=("--onboarding-app=")
    flags+=("--onboarding-permissions=")
    two_word_flags+=("--onboarding-permissions")
    local_nonpersistent_flags+=("--onboarding-permissions=")
    flags+=("--onboarding-secret=")
    two_word_flags+=("--onboarding-secret")
    local_nonpersistent_flags+=("--onboarding-secret=")
    flags+=("--onboarding-state=")
    two_word_flags+=("--onboarding-state")
    local_nonpersistent_flags+=("--onboarding-state=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_debug()
{
    last_command="cozy-stack_instances_debug"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    local_nonpersistent_flags+=("--domain=")
    flags+=("--ttl=")
    two_word_flags+=("--ttl")
    local_nonpersistent_flags+=("--ttl=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_destroy()
{
    last_command="cozy-stack_instances_destroy"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--force")
    local_nonpersistent_flags+=("--force")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_export()
{
    last_command="cozy-stack_instances_export"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    local_nonpersistent_flags+=("--domain=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_flag+=("--domain=")
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_find-oauth-client()
{
    last_command="cozy-stack_instances_find-oauth-client"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_fsck()
{
    last_command="cozy-stack_instances_fsck"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--fail-fast")
    local_nonpersistent_flags+=("--fail-fast")
    flags+=("--files-consistency")
    local_nonpersistent_flags+=("--files-consistency")
    flags+=("--index-integrity")
    local_nonpersistent_flags+=("--index-integrity")
    flags+=("--json")
    local_nonpersistent_flags+=("--json")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_import()
{
    last_command="cozy-stack_instances_import"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--directory=")
    two_word_flags+=("--directory")
    local_nonpersistent_flags+=("--directory=")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    local_nonpersistent_flags+=("--domain=")
    flags+=("--increase-quota")
    local_nonpersistent_flags+=("--increase-quota")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_flag+=("--domain=")
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_ls()
{
    last_command="cozy-stack_instances_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--available-fields")
    local_nonpersistent_flags+=("--available-fields")
    flags+=("--fields=")
    two_word_flags+=("--fields")
    local_nonpersistent_flags+=("--fields=")
    flags+=("--json")
    local_nonpersistent_flags+=("--json")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_modify()
{
    last_command="cozy-stack_instances_modify"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--blocked")
    local_nonpersistent_flags+=("--blocked")
    flags+=("--context-name=")
    two_word_flags+=("--context-name")
    local_nonpersistent_flags+=("--context-name=")
    flags+=("--deleting")
    local_nonpersistent_flags+=("--deleting")
    flags+=("--disk-quota=")
    two_word_flags+=("--disk-quota")
    local_nonpersistent_flags+=("--disk-quota=")
    flags+=("--domain-aliases=")
    two_word_flags+=("--domain-aliases")
    local_nonpersistent_flags+=("--domain-aliases=")
    flags+=("--email=")
    two_word_flags+=("--email")
    local_nonpersistent_flags+=("--email=")
    flags+=("--locale=")
    two_word_flags+=("--locale")
    local_nonpersistent_flags+=("--locale=")
    flags+=("--onboarding-finished")
    local_nonpersistent_flags+=("--onboarding-finished")
    flags+=("--public-name=")
    two_word_flags+=("--public-name")
    local_nonpersistent_flags+=("--public-name=")
    flags+=("--settings=")
    two_word_flags+=("--settings")
    local_nonpersistent_flags+=("--settings=")
    flags+=("--tos=")
    two_word_flags+=("--tos")
    local_nonpersistent_flags+=("--tos=")
    flags+=("--tos-latest=")
    two_word_flags+=("--tos-latest")
    local_nonpersistent_flags+=("--tos-latest=")
    flags+=("--tz=")
    two_word_flags+=("--tz")
    local_nonpersistent_flags+=("--tz=")
    flags+=("--uuid=")
    two_word_flags+=("--uuid")
    local_nonpersistent_flags+=("--uuid=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_refresh-token-oauth()
{
    last_command="cozy-stack_instances_refresh-token-oauth"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_set-disk-quota()
{
    last_command="cozy-stack_instances_set-disk-quota"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_set-passphrase()
{
    last_command="cozy-stack_instances_set-passphrase"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_show()
{
    last_command="cozy-stack_instances_show"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_show-app-version()
{
    last_command="cozy-stack_instances_show-app-version"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_show-db-prefix()
{
    last_command="cozy-stack_instances_show-db-prefix"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_show-swift-prefix()
{
    last_command="cozy-stack_instances_show-swift-prefix"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_token-app()
{
    last_command="cozy-stack_instances_token-app"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--expire=")
    two_word_flags+=("--expire")
    local_nonpersistent_flags+=("--expire=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_token-cli()
{
    last_command="cozy-stack_instances_token-cli"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_token-konnector()
{
    last_command="cozy-stack_instances_token-konnector"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_token-oauth()
{
    last_command="cozy-stack_instances_token-oauth"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--expire=")
    two_word_flags+=("--expire")
    local_nonpersistent_flags+=("--expire=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances_update()
{
    last_command="cozy-stack_instances_update"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--all-domains")
    local_nonpersistent_flags+=("--all-domains")
    flags+=("--context-name=")
    two_word_flags+=("--context-name")
    local_nonpersistent_flags+=("--context-name=")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    local_nonpersistent_flags+=("--domain=")
    flags+=("--force-registry")
    local_nonpersistent_flags+=("--force-registry")
    flags+=("--only-registry")
    local_nonpersistent_flags+=("--only-registry")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_instances()
{
    last_command="cozy-stack_instances"

    command_aliases=()

    commands=()
    commands+=("add")
    commands+=("auth-mode")
    commands+=("client-oauth")
    commands+=("debug")
    commands+=("destroy")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("rm")
        aliashash["rm"]="destroy"
    fi
    commands+=("export")
    commands+=("find-oauth-client")
    commands+=("fsck")
    commands+=("import")
    commands+=("ls")
    commands+=("modify")
    commands+=("refresh-token-oauth")
    commands+=("set-disk-quota")
    commands+=("set-passphrase")
    commands+=("show")
    commands+=("show-app-version")
    commands+=("show-db-prefix")
    commands+=("show-swift-prefix")
    commands+=("token-app")
    commands+=("token-cli")
    commands+=("token-konnector")
    commands+=("token-oauth")
    commands+=("update")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("updates")
        aliashash["updates"]="update"
    fi

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_jobs_purge-old-jobs()
{
    last_command="cozy-stack_jobs_purge-old-jobs"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--duration=")
    two_word_flags+=("--duration")
    local_nonpersistent_flags+=("--duration=")
    flags+=("--workers=")
    two_word_flags+=("--workers")
    local_nonpersistent_flags+=("--workers=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_jobs_run()
{
    last_command="cozy-stack_jobs_run"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--json=")
    two_word_flags+=("--json")
    local_nonpersistent_flags+=("--json=")
    flags+=("--logs")
    local_nonpersistent_flags+=("--logs")
    flags+=("--logs-verbose")
    local_nonpersistent_flags+=("--logs-verbose")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_jobs()
{
    last_command="cozy-stack_jobs"

    command_aliases=()

    commands=()
    commands+=("purge-old-jobs")
    commands+=("run")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("launch")
        aliashash["launch"]="run"
        command_aliases+=("push")
        aliashash["push"]="run"
    fi

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_konnectors_install()
{
    last_command="cozy-stack_konnectors_install"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--parameters=")
    two_word_flags+=("--parameters")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_konnectors_ls()
{
    last_command="cozy-stack_konnectors_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--parameters=")
    two_word_flags+=("--parameters")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_konnectors_run()
{
    last_command="cozy-stack_konnectors_run"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--account-id=")
    two_word_flags+=("--account-id")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--parameters=")
    two_word_flags+=("--parameters")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_konnectors_show()
{
    last_command="cozy-stack_konnectors_show"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--parameters=")
    two_word_flags+=("--parameters")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_konnectors_uninstall()
{
    last_command="cozy-stack_konnectors_uninstall"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--parameters=")
    two_word_flags+=("--parameters")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_konnectors_update()
{
    last_command="cozy-stack_konnectors_update"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--safe")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--all-domains")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--parameters=")
    two_word_flags+=("--parameters")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_konnectors()
{
    last_command="cozy-stack_konnectors"

    command_aliases=()

    commands=()
    commands+=("install")
    commands+=("ls")
    commands+=("run")
    commands+=("show")
    commands+=("uninstall")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("rm")
        aliashash["rm"]="uninstall"
    fi
    commands+=("update")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("upgrade")
        aliashash["upgrade"]="update"
    fi

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--all-domains")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--parameters=")
    two_word_flags+=("--parameters")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_serve()
{
    last_command="cozy-stack_serve"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--allow-root")
    flags+=("--appdir=")
    two_word_flags+=("--appdir")
    flags+=("--assets=")
    two_word_flags+=("--assets")
    flags+=("--couchdb-url=")
    two_word_flags+=("--couchdb-url")
    flags+=("--csp-whitelist=")
    two_word_flags+=("--csp-whitelist")
    flags+=("--dev")
    flags+=("--disable-csp")
    flags+=("--doctypes=")
    two_word_flags+=("--doctypes")
    flags+=("--downloads-url=")
    two_word_flags+=("--downloads-url")
    flags+=("--fs-default-layout=")
    two_word_flags+=("--fs-default-layout")
    flags+=("--fs-url=")
    two_word_flags+=("--fs-url")
    flags+=("--geodb=")
    two_word_flags+=("--geodb")
    flags+=("--hooks=")
    two_word_flags+=("--hooks")
    flags+=("--jobs-url=")
    two_word_flags+=("--jobs-url")
    flags+=("--konnectors-cmd=")
    two_word_flags+=("--konnectors-cmd")
    flags+=("--konnectors-oauthstate=")
    two_word_flags+=("--konnectors-oauthstate")
    flags+=("--lock-url=")
    two_word_flags+=("--lock-url")
    flags+=("--log-level=")
    two_word_flags+=("--log-level")
    flags+=("--log-syslog")
    flags+=("--mail-alert-address=")
    two_word_flags+=("--mail-alert-address")
    flags+=("--mail-disable-tls")
    flags+=("--mail-host=")
    two_word_flags+=("--mail-host")
    flags+=("--mail-noreply-address=")
    two_word_flags+=("--mail-noreply-address")
    flags+=("--mail-noreply-name=")
    two_word_flags+=("--mail-noreply-name")
    flags+=("--mail-password=")
    two_word_flags+=("--mail-password")
    flags+=("--mail-port=")
    two_word_flags+=("--mail-port")
    flags+=("--mail-username=")
    two_word_flags+=("--mail-username")
    flags+=("--mailhog")
    flags+=("--password-reset-interval=")
    two_word_flags+=("--password-reset-interval")
    flags+=("--rate-limiting-url=")
    two_word_flags+=("--rate-limiting-url")
    flags+=("--realtime-url=")
    two_word_flags+=("--realtime-url")
    flags+=("--sessions-url=")
    two_word_flags+=("--sessions-url")
    flags+=("--subdomains=")
    two_word_flags+=("--subdomains")
    flags+=("--vault-decryptor-key=")
    two_word_flags+=("--vault-decryptor-key")
    flags+=("--vault-encryptor-key=")
    two_word_flags+=("--vault-encryptor-key")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_settings()
{
    last_command="cozy-stack_settings"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_status()
{
    last_command="cozy-stack_status"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_swift_get()
{
    last_command="cozy-stack_swift_get"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_swift_ls()
{
    last_command="cozy-stack_swift_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_swift_ls-layouts()
{
    last_command="cozy-stack_swift_ls-layouts"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--show-domains")
    local_nonpersistent_flags+=("--show-domains")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_swift_put()
{
    last_command="cozy-stack_swift_put"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--content-type=")
    two_word_flags+=("--content-type")
    local_nonpersistent_flags+=("--content-type=")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_swift_rm()
{
    last_command="cozy-stack_swift_rm"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_swift()
{
    last_command="cozy-stack_swift"

    command_aliases=()

    commands=()
    commands+=("get")
    commands+=("ls")
    commands+=("ls-layouts")
    commands+=("put")
    commands+=("rm")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("delete")
        aliashash["delete"]="rm"
    fi

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_triggers_launch()
{
    last_command="cozy-stack_triggers_launch"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_triggers_ls()
{
    last_command="cozy-stack_triggers_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_triggers_show-from-app()
{
    last_command="cozy-stack_triggers_show-from-app"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_triggers()
{
    last_command="cozy-stack_triggers"

    command_aliases=()

    commands=()
    commands+=("launch")
    commands+=("ls")
    commands+=("show-from-app")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--domain=")
    two_word_flags+=("--domain")
    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_version()
{
    last_command="cozy-stack_version"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_cozy-stack_root_command()
{
    last_command="cozy-stack"

    command_aliases=()

    commands=()
    commands+=("apps")
    commands+=("assets")
    commands+=("bug")
    commands+=("completion")
    commands+=("config")
    commands+=("doc")
    commands+=("features")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("feature")
        aliashash["feature"]="features"
    fi
    commands+=("files")
    commands+=("fixer")
    commands+=("instances")
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        command_aliases+=("instance")
        aliashash["instance"]="instances"
    fi
    commands+=("jobs")
    commands+=("konnectors")
    commands+=("serve")
    commands+=("settings")
    commands+=("status")
    commands+=("swift")
    commands+=("triggers")
    commands+=("version")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--admin-host=")
    two_word_flags+=("--admin-host")
    flags+=("--admin-port=")
    two_word_flags+=("--admin-port")
    flags+=("--config=")
    two_word_flags+=("--config")
    two_word_flags+=("-c")
    flags+=("--host=")
    two_word_flags+=("--host")
    flags+=("--port=")
    two_word_flags+=("--port")
    two_word_flags+=("-p")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

__start_cozy-stack()
{
    local cur prev words cword
    declare -A flaghash 2>/dev/null || :
    declare -A aliashash 2>/dev/null || :
    if declare -F _init_completion >/dev/null 2>&1; then
        _init_completion -s || return
    else
        __cozy-stack_init_completion -n "=" || return
    fi

    local c=0
    local flags=()
    local two_word_flags=()
    local local_nonpersistent_flags=()
    local flags_with_completion=()
    local flags_completion=()
    local commands=("cozy-stack")
    local must_have_one_flag=()
    local must_have_one_noun=()
    local last_command
    local nouns=()

    __cozy-stack_handle_word
}

if [[ $(type -t compopt) = "builtin" ]]; then
    complete -o default -F __start_cozy-stack cozy-stack
else
    complete -o default -o nospace -F __start_cozy-stack cozy-stack
fi

# ex: ts=4 sw=4 et filetype=sh

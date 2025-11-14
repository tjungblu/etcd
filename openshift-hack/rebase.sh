#!/bin/bash

# READ FIRST BEFORE USING THIS SCRIPT
#
# This script requires jq, git, podman and bash to work properly (dependencies are checked for you).
# The Github CLI "gh" is optional, but convenient to create a pull request automatically at the end.
#
# The usage is described in /REBASE.openshift.md.

# validate input args --etcd-tag=v3.6.6 --jira-id=666
etcd_tag=""
jira_id=""

usage() {
  echo "Available arguments:"
  echo "  --etcd-tag            (required) Example: --etcd-tag=v3.6.6"
  echo "  --jira-id             (optional) creates new PR against openshift/etcd: Example: --jira-id=666"
}

for i in "$@"; do
  case $i in
  --etcd-tag=*)
    etcd_tag="${i#*=}"
    shift
    ;;
  --jira-id=*)
    jira_id="${i#*=}"
    shift
    ;;
  *)
    usage
    exit 1
    ;;
  esac
done

if [ -z "${etcd_tag}" ]; then
  echo "Required argument missing: --etcd-tag"
  echo ""
  usage
  exit 1
fi

echo "Processed arguments are:"
echo "--etcd_tag=${etcd_tag}"
echo "--jira_id=${jira_id}"

# prerequisites (check git, podman, ... is present)
if ! command -v git &>/dev/null; then
  echo "git not installed, exiting"
  exit 1
fi

if ! command -v jq &>/dev/null; then
  echo "jq not installed, exiting"
  exit 1
fi

if ! command -v podman &>/dev/null; then
  echo "podman not installed, exiting"
  exit 1
fi

# make sure we're in "etcd" dir, but we also allow openshift-etcd
if [[ $(basename "$PWD") != "etcd" && $(basename "$PWD") != "openshift-etcd" ]]; then
  echo "Not in etcd dir, exiting"
  exit 1
fi

origin=$(git remote get-url origin)
if [[ "$origin" =~ .*etcd-io/etcd.* || "$origin" =~ .*openshift/etcd.* ]]; then
  echo "cannot rebase against etcd-io/etcd or openshift/etcd! found: ${origin}, exiting"
  exit 1
fi

# fetch remote https://github.com/etcd-io/etcd
git remote add upstream git@github.com:etcd-io/etcd.git
git fetch upstream --tags -f
# fetch remote https://github.com/openshift/etcd
git remote add openshift git@github.com:openshift/etcd.git
git fetch openshift

# clean checkout of the remote openshift release
git checkout -b "rebase_tmp_${etcd_tag}"
git branch -D "main"
git checkout --track "openshift/main"
git pull openshift main

# that should give us the latest (or highest version) etcd tag
# This is a bit experimental for the future, but works across all the current release branches
etcd_forkpoint=$(git tag --merged | sort -V | tail -2 | head -1)
if [[ "$etcd_forkpoint" == "$etcd_tag" ]]; then
  echo "forkpoint $etcd_forkpoint matches given etcd tag, no rebase necessary"
  exit 1
fi

echo "running: \`git rebase --rebase-merges --fork-point $etcd_forkpoint $etcd_tag\`"
git rebase --rebase-merges --fork-point "$etcd_forkpoint" "$etcd_tag"
echo "running: \`git merge main\`"
git merge main

# shellcheck disable=SC2181
if [ $? -eq 0 ]; then
  echo "No conflicts detected. Automatic merge looks to have succeeded"
else
  # commit conflicts
  git commit -a
  # resolve conflicts
  git status
  # TODO(tjungblu): we follow-up with a more automated approach:
  # - 2/3s of conflicts stem from go.mod/sum, which can be resolved deterministically
  # - the large majority of the remainder are vendor/generation conflicts
  # - only very few cases require manual intervention due to conflicting business logic
  echo "Resolve conflicts manually in another terminal, only then continue"

  # wait for user interaction
  read -n 1 -s -r -p "PRESS ANY KEY TO CONTINUE"

  # TODO(tjungblu): verify that the conflicts have been resolved
  git commit -am "UPSTREAM: <drop>: manually resolve conflicts"
fi

# ensure we always use the correct openshift release + golang combination
img_tag=$(cat Dockerfile.art | head -n1 | sed -n 's/^FROM .*:\([^ ]*\) AS builder/\1/p')
echo "> go mod tidy"
podman run -it --rm -v "$(pwd):/go/etcd:Z" \
  --workdir=/go/etcd \
  "registry.ci.openshift.org/openshift/release:$img_tag" \
  go mod tidy

# shellcheck disable=SC2181
if [ $? -ne 0 ]; then
  echo "go mod tidy failed, is any dependency missing?"
  exit 1
fi

git add -A
git commit -m "UPSTREAM: <drop>: go mod tidy"

remote_branch="rebase-$etcd_tag"
git push origin "main:$remote_branch"

XY=$(echo "$etcd_tag" | sed -E "s/v(1\.[0-9]+)\.[0-9]+/\1/")
ver=$(echo "$etcd_tag" | sed "s/\.//g")
link="https://github.com/etcd-io/etcd/blob/master/CHANGELOG/CHANGELOG-$XY.md#$ver"
if [ -n "${jira_id}" ]; then
  if command -v gh &>/dev/null; then
    XY=$(echo "$etcd_tag" | sed -E "s/v(1\.[0-9]+)\.[0-9]+/\1/")
    ver=$(echo "$etcd_tag" | sed "s/\.//g")
    link="https://github.com/etcd-io/etcd/blob/master/CHANGELOG/CHANGELOG-$XY.md#$ver"

    # opens a web browser, because we can't properly create PRs against remote repositories with the GH CLI (yet):
    # https://github.com/cli/cli/issues/2691
    gh pr create \
      --title "OCPBUGS-$jira_id: Rebase $etcd_tag" \
      --body "CHANGELOG $link" \
      --web

  fi
fi

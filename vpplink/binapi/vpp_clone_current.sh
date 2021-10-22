#!/bin/bash
set -e

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
CACHE_DIR=$SCRIPTDIR/.cherries-cache

function green () { printf "\e[0;32m$1\e[0m\n" >&2 ; }
function blue () { printf "\e[0;34m$1\e[0m\n" >&2 ; }
function grey () { printf "\e[0;37m$1\e[0m\n" >&2 ; }
function red () { printf "\e[0;31m$1\e[0m\n" >&2 ; }
function commit_exists () { git show $1 > /dev/null 2>&1 && echo "true" || echo "false" ; }
function commit_exists_f () { [ -f $1 ] && commit_exists $(cat $1) || echo "false" ; }

function exit_and_print_help ()
{
	msg=$1
	refs=$2
	red "${msg} failed ! Patch can be found at :"
	red "https://gerrit.fd.io/r/c/vpp/+/${refs#refs/changes/*/}"
	echo
	red "you can use 'cherry-wipe' if you fear this is a caching issue"
	exit 1
}

function git_get_commit_from_refs ()
{
	refs=$1
	mkdir -p $CACHE_DIR
	CACHE_F=$CACHE_DIR/$(echo $refs |sed s@refs/changes/@@g |sed s@/@_@g)
	if $(commit_exists_f $CACHE_F); then
		COMMIT_HASH=$(cat $CACHE_F)
		green "Using cached $COMMIT_HASH"
	else
		green "Fetching $refs"
		git fetch "https://gerrit.fd.io/r/vpp" ${refs}
		COMMIT_HASH=$(git log FETCH_HEAD -1 --pretty=%H)
	fi
	echo $COMMIT_HASH > $CACHE_F
	echo $COMMIT_HASH
}

function git_cherry_pick ()
{
	refs=$1
    blue "Cherry picking $refs..."
	COMMIT_HASH=$(git_get_commit_from_refs $refs)
	git cherry-pick $COMMIT_HASH || exit_and_print_help "Cherry pick" $refs
	git commit --amend -m "gerrit:${refs#refs/changes/*/} $(git log -1 --pretty=%B)"
}

function git_revert ()
{
	refs=$1
    blue "Reverting $refs..."
	COMMIT_HASH=$(git_get_commit_from_refs $refs)
	git revert --no-edit $COMMIT_HASH || exit_and_print_help "Revert" $refs
	git commit --amend -m "gerrit:revert:${refs#refs/changes/*/} $(git log -1 --pretty=%B)"
}

function git_clone_cd_and_reset ()
{
	VPP_DIR=$1
	VPP_COMMIT=$2
	if [ -z "$VPP_DIR" ]; then
		red "Please provide the VPP repository path"
		exit 1
	fi
	if [ ! -d $VPP_DIR/.git ]; then
		if [ x$VPP_DIR == x/ ]; then
			red "Beware, trying to remove '$VPP_DIR'"
			exit 1
		fi
		rm -rf $VPP_DIR
		green "Cloning VPP..."
		git clone "https://gerrit.fd.io/r/vpp" $VPP_DIR
	fi
	cd $VPP_DIR
	if ! $(commit_exists $VPP_COMMIT); then
		green "Fetching most recent VPP..."
		git fetch "https://gerrit.fd.io/r/vpp"
	fi
	git reset --hard ${VPP_COMMIT}
}

# --------------- Things to cherry pick ---------------

git_clone_cd_and_reset "$1" 5b9cb0d275959aeb7f4e9dfb7353b638ec9a2e5a

git_cherry_pick refs/changes/12/33312/4 # 33312: sr: fix srv6 definition of behavior associated to a LocalSID | https://gerrit.fd.io/r/c/vpp/+/33312
git_cherry_pick refs/changes/13/34713/3 # 34713: vppinfra: improve & test abstract socket | https://gerrit.fd.io/r/c/vpp/+/34713
git_cherry_pick refs/changes/71/32271/15 # 32271: memif: add support for ns abstract sockets | https://gerrit.fd.io/r/c/vpp/+/32271
git_cherry_pick refs/changes/34/34734/2 # 34734: memif: autogenerate socket_ids | https://gerrit.fd.io/r/c/vpp/+/34734
git_cherry_pick refs/changes/26/34726/1 # 34726: interface: add buffer stats api | https://gerrit.fd.io/r/c/vpp/+/34726
git_cherry_pick refs/changes/57/34757/1 # 34757: tap: add num_tx_queues API | https://gerrit.fd.io/r/c/vpp/+/34757
git_cherry_pick refs/changes/74/34674/1 # 34674: vppinfra: add new bihash exports | https://gerrit.fd.io/r/c/vpp/+/34674

git_cherry_pick refs/changes/49/31449/8 # 31449: cnat: dont compute offloaded cksums | https://gerrit.fd.io/r/c/vpp/+/31449
git_cherry_pick refs/changes/08/34108/3 # 34108: cnat: flag to disable rsession | https://gerrit.fd.io/r/c/vpp/+/34108
git_cherry_pick refs/changes/21/32821/4 # 32821: cnat: add ip/client bihash | https://gerrit.fd.io/r/c/vpp/+/32821
git_cherry_pick refs/changes/48/29748/3 # 29748: cnat: remove rwlock on ts | https://gerrit.fd.io/r/c/vpp/+/29748
git_cherry_pick refs/changes/52/34552/4 # 34552: cnat: add single lookup | https://gerrit.fd.io/r/c/vpp/+/34552
git_cherry_pick refs/changes/88/31588/3 # 31588: cnat: [WIP] no k8s maglev from pods | https://gerrit.fd.io/r/c/vpp/+/31588

# --------------- Dedicated plugins ---------------
git_cherry_pick refs/changes/64/33264/7 # 33264: pbl: Port based balancer | https://gerrit.fd.io/r/c/vpp/+/33264
git_cherry_pick refs/changes/83/28083/20 # 28083: acl: acl-plugin custom policies |  https://gerrit.fd.io/r/c/vpp/+/28083
git_cherry_pick refs/changes/13/28513/24 # 25813: capo: Calico Policies plugin | https://gerrit.fd.io/r/c/vpp/+/28513
# --------------- Dedicated plugins ---------------

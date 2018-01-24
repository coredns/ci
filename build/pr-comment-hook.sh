#!/bin/bash

set -e
export COREDNSFORK='coredns'
export COREDNSPROJ='coredns'
export COREDNSREPO="github.com/${COREDNSFORK}"
export COREDNSPATH="github.com/${COREDNSPROJ}"
export HOME=/root

export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin:/root/go/bin

export GHTOKEN=$(cat /root/.ghtoken)

# We receive all json in one giant string in the env var $PAYLOAD.
if [[ -z ${PAYLOAD} ]]; then
    exit 1
fi

# Only trigger on comment creation
action=$(echo ${PAYLOAD} | jq '.action' | tr -d "\n\"")
if [[ "${action}" != "created" ]]; then
    exit 0
fi

# PRs are Issues with a pull_request section.
# If there is no pull_request section, then this is not a PR, we can exit
pull_url=$(echo ${PAYLOAD} | jq '.issue.pull_request.url' | tr -d "\n\"")
if [[ -z ${pull_url} ]]; then
    exit 0
fi

# Get the PR number
export PR=$(echo ${PAYLOAD} | jq '.issue.number' | tr -d "\n\"")

# Get Repo Name
export SOURCEPROJ=$(echo ${PAYLOAD} | jq '.repository.name' | tr -d "\n\"")

# Get SHA for test head commit (used to update PR status)
export SHA=$(curl -s --user ${GHTOKEN} -X GET https://api.github.com/repos/${COREDNSFORK}/${SOURCEPROJ}/pulls/${PR} | jq '.head.sha' | tr -d "\n\"")
echo curl -s --user XXXXXXXX -X GET https://api.github.com/repos/${COREDNSFORK}/${SOURCEPROJ}/pulls/${PR}
echo Commit SHA: $SHA

# Create temporary workspace and set GOPATH
workdir=$(mktemp -d)
export GOPATH=$workdir

# Set up a clean up on exit
function rmWorkdir {
    rm -rf ${workdir}
}
trap rmWorkdir EXIT

function postComment {
    body=$1
    curl --user $GHTOKEN -X POST --data "{\"body\":\"$body\"}" https://api.github.com/repos/${COREDNSFORK}/${SOURCEPROJ}/issues/${PR}/comments | jq '.id'
}

function updateComment {
    body=$1
    curl --user $GHTOKEN -X POST --data "{\"body\":\"$body\"}" https://api.github.com/repos/${COREDNSFORK}/${SOURCEPROJ}/issues/comments/${STATUSID}
}

function postStatus {
    state=$1
    descr=$2
    url=$3
    context="coredns/ci"
    curl --user $GHTOKEN -X POST --data "{\"state\":\"$state\",\"description\":\"$descr\",\"target_url\":\"$url\",\"context\":\"$context\"}" https://api.github.com/repos/${COREDNSFORK}/${SOURCEPROJ}/statuses/${SHA}
}

function lockFail {
    postStatus "error" "Integration test could not start. (timeout waiting for lock)" "$LOG_URL"
    exit 1
}

# Get the contents of the comment
body=$(echo ${PAYLOAD} | jq '.comment.body' | tr -d "\n\"")
case "${body}" in
    */integration*)

    [[ "${body}" =~ \/integration-cipr([0-9]+) ]] && export CIPR=${BASH_REMATCH[1]}

    export DEPLOYMENTPATH=${GOPATH}/src/${COREDNSPATH}/deployment
    export LOG_URL="https://drone.coredns.io/log/view.html?pr=${SHA}"
    postStatus "pending" "Integration test requested." ""

    # Check lock
    exec 9>/var/lock/integration-test
    flock -w 300 9 || lockFail

    # Setup log and post status + log link to PR
    export logpath=/var/www/log/${SHA}.txt
    touch $logpath  2>&1
    echo "############### Integration Test ${COREDNSREPO}:PR${PR} Workdir: ${GOPATH} #######################" > $logpath
    chown www-data: $logpath 2>&1

    # Get ci code
    mkdir -p ${GOPATH}/src/${COREDNSPATH}
    cd ${GOPATH}/src/${COREDNSPATH}
    git clone https://${COREDNSREPO}/ci.git
    cd ci
    if [[ -n "$CIPR" ]] ; then
      echo "Fetching CI PR $CIPR..." >> $logpath 2>&1
      git fetch --depth 1 origin pull/${CIPR}/head:pr-${CIPR} >> $logpath 2>&1
      git checkout pr-${CIPR}  >> $logpath 2>&1
    fi

    # Set up a finish & clean up on exit
    function finishIntegrationTest {
        make clean-k8s >> $logpath 2>&1

        pass=$(cat $logpath | grep "^\-\-\- PASS:" | wc -l)
        fail=$(cat $logpath | grep "^\-\-\- FAIL:" | wc -l)
        subpass=$(cat $logpath | grep "^    \-\-\- PASS:" | wc -l)
        subfail=$(cat $logpath | grep "^    \-\-\- FAIL:" | wc -l)
	total=$((pass + fail))
	totalsub=$((subpass + subfail))

        summary="passed ${total} tests and ${totalsub} subtests."
	state="success"
        if [[ "${total}" == "0" ]]; then
            summary="failed to complete."
	    state="error"
        fi
        if [[ "${fail}" != "0" ]]; then
            summary="failed ${fail}/${total} tests and ${subfail}/${totalsub} subtests."
	    state="failure"
        fi

        # Post result to pr
        postStatus "$state" "Integration test $summary" "$LOG_URL"
        rmWorkdir
    }
    trap finishIntegrationTest EXIT

    # Do integration setup and test
    postStatus "pending" "Integration test in progress." "$LOG_URL"
    export K8S_VERSION='v1.7.5'
    SECONDS=0
    make test-${SOURCEPROJ} >> /var/www/log/${SHA}.txt 2>&1

  ;;
esac

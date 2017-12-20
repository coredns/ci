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

# Create temporary workspace and set GOPATH
workdir=$(mktemp -d)
export GOPATH=$workdir

# Set up a clean up on exit
function rmWorkdir {
    rm -rf ${workdir}
}
trap rmWorkdir EXIT

function postStatus {
    body=$1
    curl --user $GHTOKEN -X POST --data "{\"body\":\"$body\"}" https://api.github.com/repos/${COREDNSFORK}/${SOURCEPROJ}/issues/${PR}/comments | jq '.id'
}

function updateStatus {
    body=$1
    curl --user $GHTOKEN -X POST --data "{\"body\":\"$body\"}" https://api.github.com/repos/${COREDNSFORK}/${SOURCEPROJ}/issues/comments/${STATUSID}
}

function lockFail {
    updateStatus "Integration test could not start. (timeout waiting for lock)"
    exit 1
}

# Get the contents of the comment
body=$(echo ${PAYLOAD} | jq '.comment.body' | tr -d "\n\"")
case "${body}" in
    */integration*)

    export SOURCEPROJ=$(echo ${PAYLOAD} | jq '.repository.name' | tr -d "\n\"")
    export DEPLOYMENTPATH=${GOPATH}/src/${COREDNSPATH}/deployment
    export STATUSID=$(postStatus "Integration test request received.")

    # Check lock
    exec 9>/var/lock/integration-test
    flock -w 300 9 || lockFail

    # Setup log and post status + log link to PR
    touch /var/www/log/${PR}.txt  2>&1
    echo "############### Integration Test ${COREDNSREPO}:PR${PR} Workdir: ${GOPATH} #######################" > /var/www/log/${PR}.txt
    chown www-data: /var/www/log/${PR}.txt 2>&1
    updateStatus "Integration test started. <a href='https://drone.coredns.io/log/view.html?pr=${PR}'>View Log</a>"

    # Get ci code
    mkdir -p ${GOPATH}/src/${COREDNSPATH}
    cd ${GOPATH}/src/${COREDNSPATH}
    git clone https://${COREDNSREPO}/ci.git
    cd ci

	# Set up a finish & clean up on exit
    function finishIntegrationTest {
        make clean-k8s >> /var/www/log/${PR}.txt 2>&1
        # Post result to pr
        pass=$(cat /var/www/log/${PR}.txt | grep "^\-\-\- PASS:" | wc -l)
        fail=$(cat /var/www/log/${PR}.txt | grep "^\-\-\- FAIL:" | wc -l)
        subpass=$(cat /var/www/log/${PR}.txt | grep "^    \-\-\- PASS:" | wc -l)
        subfail=$(cat /var/www/log/${PR}.txt | grep "^    \-\-\- FAIL:" | wc -l)
        if [[ "${fail}" != "0" ]]; then
            summary="FAIL"
        fi
        printf -v summary '\\n\\n
        |          | Pass   | Fail   |\\n
        | -------- | ------ | ------ |\\n
        | Tests    | %s     | %s     |\\n
        | Subtests | %s     | %s     |\\n' "${pass}" "${fail}" "${subpass}" "${subfail}"
        summary=$(echo $summary | tr -d '\n')
        updateStatus "Integration test $status. Run time ${SECONDS} seconds. <a href='https://drone.coredns.io/log/view.html?pr=${PR}'>View Log</a>$summary"
        rmWorkdir
    }
    trap finishIntegrationTest EXIT

    # Do integration setup and test
    export K8S_VERSION='v1.7.5'
    status="FAIL"
    SECONDS=0
    make test-${SOURCEPROJ} >> /var/www/log/${PR}.txt 2>&1 && status="PASS"

  ;;
esac
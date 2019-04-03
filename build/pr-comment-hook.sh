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

# Get Event Type
pull_request=$(echo ${PAYLOAD} | jq '.pull_request' | tr -d "\n\"")
comment=$(echo ${PAYLOAD} | jq '.comment' | tr -d "\n\"")
if [[ "${pull_request}" != "null" ]]; then
    # Pull Request Event
    echo PULL REQUEST EVENT
    event_type=pullrequest
elif [[ "${comment}" != "null" ]]; then
    # Issue Comment Event
    echo ISSUE COMMENT EVENT
    event_type=comment
else
    # Unhandled Event
    echo UNHANDLED EVENT TYPE
    exit 0
fi

# Get Repo Name
export SOURCEPROJ=$(echo ${PAYLOAD} | jq '.repository.name' | tr -d "\n\"")
export SOURCEPROJFULL=$(echo ${PAYLOAD} | jq '.repository.full_name' | tr -d "\n\"")

# Parse Issue Comment Event
if [[ ${event_type} == "comment" ]]; then
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
  if [[ "${PR}" == "null" ]]; then
    exit 1
  fi

  # Get SHA for test head commit (used to update PR status)
  export SHA=$(curl -s --user ${GHTOKEN} -X GET https://api.github.com/repos/${SOURCEPROJFULL}/pulls/${PR} | jq '.head.sha' | tr -d "\n\"")
  if [[ "${SHA}" == "null" ]]; then
    exit 1
  fi
fi

# Parse Pull Request Event
if [[ ${event_type} == "pullrequest" ]]; then
  # Only trigger on commit synchronization
  action=$(echo ${PAYLOAD} | jq '.action' | tr -d "\n\"")
  if [[ "${action}" != "synchronize" ]] && [[ "${action}" != "opened" ]]; then
      exit 0
  fi

  # Get the PR number
  export PR=$(echo ${PAYLOAD} | jq '.number' | tr -d "\n\"")
  if [[ "${PR}" == "null" ]]; then
    exit 1
  fi

  # Get SHA for test head commit (used to update PR status)
  export SHA=$(echo ${PAYLOAD} | jq '.pull_request.head.sha' | tr -d "\n\"")
  if [[ "${SHA}" == "null" ]]; then
    exit 1
  fi
fi


function rmWorkdir {
    rm -rf ${workdir}
}

function postComment {
    msg=$1
    curl --user $GHTOKEN -X POST --data "{\"body\":\"$msg\"}" https://api.github.com/repos/${SOURCEPROJFULL}/issues/${PR}/comments | jq '.id'
}

function updateComment {
    msg=$1
    curl --user $GHTOKEN -X POST --data "{\"body\":\"$msg\"}" https://api.github.com/repos/${SOURCEPROJFULL}/issues/comments/${STATUSID}
}

function postStatus {
    state=$1
    descr=$2
    url=$3
    context="coredns/ci"
    curl --user $GHTOKEN -X POST --data "{\"state\":\"$state\",\"description\":\"$descr\",\"target_url\":\"$url\",\"context\":\"$context\"}" https://api.github.com/repos/${SOURCEPROJFULL}/statuses/${SHA}
}

function lockFail {
    postStatus "error" "Integration test could not start. (timeout waiting for lock)" "$LOG_URL"
    exit 1
}

function startIntegrationTest {
    # Create temporary workspace and set GOPATH
    workdir=$(mktemp -d)
    export GOPATH=$workdir
    trap rmWorkdir EXIT

    export DEPLOYMENTPATH=${GOPATH}/src/${COREDNSPATH}/deployment
    export LOG_URL="https://drone.coredns.io/log/view.html?pr=${SHA}"
    postStatus "pending" "Integration test requested, waiting for lock." ""

    # Check lock
    exec 9>/var/lock/integration-test
    flock -w 600 9 || lockFail

    # Setup log and post status + log link to PR
    export logpath=/var/www/log/${SHA}.txt
    touch $logpath  2>&1
    echo "############### Integration Test REPO:${COREDNSREPO} COMMIT:${SHA} Workdir: ${GOPATH} #######################" > $logpath
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

    # Do integration setup and test
    postStatus "pending" "Integration test in progress." "$LOG_URL"
    trap finishIntegrationTest EXIT
    SECONDS=0
    make test-${SOURCEPROJ} >> /var/www/log/${SHA}.txt 2>&1
}

function finishIntegrationTest {
  make clean-k8s >> $logpath 2>&1

  pass=$(cat $logpath | grep "^\-\-\- PASS:" | wc -l)
  fail=$(cat $logpath | grep "^\-\-\- FAIL:" | wc -l)
  subpass=$(cat $logpath | grep "^    \-\-\- PASS:" | wc -l)
  subfail=$(cat $logpath | grep "^    \-\-\- FAIL:" | wc -l)
  kills=$(cat $logpath | grep "^*** Test killed " | wc -l)
  fail=$((fail + kills))
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
  echo "# Test Summary" >> $logpath
  echo "Integration test $summary" >> $logpath
  rmWorkdir
}


# Process Issue-Comment Event
#
if [[ ${event_type} == "comment" ]]; then
  # Get the contents of the comment
  body=$(echo ${PAYLOAD} | jq '.comment.body' | tr -d "\n\"")
  case "${body}" in

    */integration*)
      # Look for -ciprXX option
      [[ "${body}" =~ \/integration-cipr([0-9]+) ]] && export CIPR=${BASH_REMATCH[1]}
      # Do integrtation test
      startIntegrationTest
    ;;

    */echo*)
      # Echo back via a comment
      export STATUSID=$(postComment "ECHO ...")
      sleep 5
      updateComment "ECHO ... echo"
    ;;

  esac
fi

# Process Pull Request Event
#
if [[ ${event_type} == "pullrequest" ]]; then
  startIntegrationTest
fi

socket = undefined;
$(window).on('beforeunload', function(event) {
    $.ajax({ type: 'post', url: 'http://' + workerIP + '/session?userID=' + userID + '&sessionID=' + sessionID });
});

function initWS() {
    socket = new WebSocket("ws://" + workerIP + "/ws?userID=" + userID + '&sessionID=' + sessionID);
    statusHTML = $('#status');

    socket.onopen = onOpen;
    socket.onclose = onClose;
    socket.onmessage = onMessage;
}

function onOpen() {
    initCRDT();
}

function onClose() {
    $.ajax({ type: 'post', url: 'http://' + workerIP + '/session?userID=' + userID + '&sessionID=' + sessionID });
}

function onMessage(_msg) {
    const element = JSON.parse(_msg.data);
    if (element.hasOwnProperty('Job')) {
        matchLog(element);
    } else {
        handleRemoteOperation(element);
    }
}

function matchLog(log) {
    if (log.Job.SessionID == sessionID) {
        var isExist = false;
        for (var i = 0; i < jobIDs.length; i++) {
            if (jobIDs[i] == log.Job.JobID) {
                isExist = true;
                var logOutput = document.getElementById(log.Job.JobID);
                logOutput.addEventListener('click', function(e) {
                    e.preventDefault();
                    logClicked(log);
                }, false);
            }
        }
        if (!isExist) {
            $("#logList").prepend("<li><a href=# id=" + log.Job.JobID + ">" + log.Job.JobID + "</a></li>")
            var logOutput = document.getElementById(log.Job.JobID);
            logOutput.addEventListener('click', function(e) {
                e.preventDefault();
                logClicked(log);
            }, false);
        }
    }
}

function logClicked(log) {
    editor.setValue(log.Job.Snippet);
    editor.setOption("readOnly", true);
    str = log.Output.replace(/(?:\r\n|\r|\n)/g, '<br />');
    document.getElementById('outputBox').innerHTML = str;
    document.getElementById("snipTitle").style.color = '#d00'
    document.getElementById('snipTitle').innerHTML = "Snippet: READ ONLY"
}

function sendElement(id) {
    const _element = CRDT.get(id);
    const element = {
        SessionID: sessionID,
        ClientID: userID,
        ID: _element.id,
        PrevID: _element.prev,
        Text: _element.val,
        Deleted: _element.del
    };

    socket.send(JSON.stringify(element));
}
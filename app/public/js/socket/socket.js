socket = undefined;
unload = false;

$(window).on('beforeunload', function(event) {
    unload = true;

    closeSession();
});

function initWS() {
    socket = new WebSocket("ws://" + workerIP + "/ws?userID=" + userID + '&sessionID=' + sessionID);
    statusHTML = $('#status');

    socket.onopen = onOpen;
    socket.onclose = onClose;
    socket.onmessage = onMessage;
    socket.onerror = onError;
}

function onOpen() {
    if (recovering) {
        sendCachedElements();
        recovering = false;
    } else {
        initCRDT();
    }
}

function onClose(e) {
    if (!unload) {
        closeSession();
        setTimeout(recover, 3000);
    }
}

function onError(e) {
    if (!unload) {
        closeSession();
        setTimeout(recover, 3000);
    }
}

function onMessage(_msg) {
    const element = JSON.parse(_msg.data);

    if (element == undefined) {
        return;
    } else if (element.hasOwnProperty('Job')) {
        matchLog(element);
    } else {
        handleRemoteOperation(element);
    }
}

function sendElement(_element) {
    if (socket.readyState != 1) return;

    const element = {
        SessionID: sessionID,
        ClientID: userID,
        ID: _element.id,
        PrevID: _element.prev,
        Text: _element.val,
        Deleted: _element.del
    };

    socket.send(JSON.stringify(element));

    if (debugMode) {
        if (element.Deleted) {
            console.log("=============DELETE===========\n" +
                "ID: " + element.ID + "\n" +
                "PREV ID: " + element.PrevID + "\n" +
                "TEXT: " + element.Text + "\n" +
                "=============================")
        } else {
            console.log("=============INSERT===========\n" +
                "ID: " + element.ID + "\n" +
                "PREV ID: " + element.PrevID + "\n" +
                "TEXT: " + element.Text + "\n" +
                "=============================")
        }

    }
}

function sendElementByID(id) {
    const _element = CRDT.get(id);
    sendElement(_element);
}

function sendCachedElements() {
    cache.forEach(function(element) {
        sendElement(element);
    });
}

function closeSession() {
    $.ajax({
        type: 'post',
        url: 'http://' + workerIP + '/session?userID=' + userID + '&sessionID=' + sessionID
    });

    if (socket.readyState == 0 || socket.readyState == 1) socket.close();
}

function getWorker(cb) {
    $.ajax({
        type: 'post',
        url: '/register',
        dataType: 'json',
        data: $('#register').serialize(),
        success: cb
    });
}

function register() {
    getWorker(function(data) {
        if (data.WorkerIP.length == 0) {
            alert("No available Workers, please try again later")
        } else {
            workerIP = data.WorkerIP;

            initWS();
            openEditor();
        }
    });
}

function recover() {
    recovering = true;

    getWorker(function(data) {
        if (data.WorkerIP.length == 0) {
            alert("Lost worker connection! Please re-try later.")
        } else {
            workerIP = data.WorkerIP;

            $.ajax({
                type: 'get',
                url: 'http://' + workerIP + '/recover?sessionID=' + sessionID,
                success: function(data) {
                    if (data != null && data.length > 0) {
                        data.Session.forEach(function(element) {
                            handleRemoteOperation(element);
                        });
                    }

                    if (data.hasOwnProperty('LogRecord')) {
                        $("#logList").empty();
                        
                        const logs = data.LogRecord
                        if (logs != null) {
                            for (var i = 0; i < logs.length; i++) {
                            if (!jobIDs.includes(logs[i].Job.JobID)) {
                                jobIDs.push(logs[i].Job.JobID);
                                $("#logList").prepend("<li><a href=# id=" + logs[i].Job.JobID + ">" + logs[i].Job.JobID + "</a></li>")
                                if (logs[i].Job.Done) {
                                    var logOutput = document.getElementById(logs[i].Job.JobID);
                                    var _log = logs[i];
                                    (function(_log) {
                                        logOutput.addEventListener('click', function(e) {
                                            e.preventDefault();
                                            logClicked(_log);
                                        }, false);
                                    })(_log);
                                }
                            }
                        }
                        }
                    }

                    initWS();
                },
                error: function() {
                    alert("Lost worker connection! Please re-try later.")
                }
            });
        }
    });
}
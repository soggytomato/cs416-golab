socket = undefined;
unload = false;
disconnectAlerted = false

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
        initSession();
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
            recoverFail();

            setTimeout(recover(), 2000);
        } else {
            workerIP = data.WorkerIP;

            $.ajax({
                type: 'get',
                url: 'http://' + workerIP + '/recover?sessionID=' + sessionID,
                success: function(data) {
                    recoverSuccess();

                    if (data != null && data.length > 0) {
                        data.Session.forEach(function(element) {
                            handleRemoteOperation(element);
                        });
                    }

                    if (data.hasOwnProperty('LogRecord')) {
                        const logs = data.LogRecord
                        if (logs != null) {
                            for (var i = 0; i < logs.length; i++) {
                                if (jobIDs.get(logs[i].Job.JobID.toString()) == undefined) {
                                    jobIDs.set(logs[i].Job.JobID, logs[i].Job.Done)
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

                    if (recoverLog) {
                        $.ajax({
                            type: 'post',
                            url: "http://" + workerIP + '/execute',
                            dataType: 'json',
                            data: recoverLog,
                            success: function(data) {
                                jobIDs.set(data.JobID, false);
                                //jobIDs.push(data.JobID);
                                $("#logList").prepend("<li><a href=# id=" + data.JobID + ">" + data.JobID + "</a></li>")
                                recoverLog = "";
                                console.log(recoverLog);
                            }
                        })
                    }

                    initWS();
                },
                error: function() {
                    recoverFail();
                }
            });
        }
    });
}

function recoverSuccess() {
    alert("Worker connection re-established!");
    disconnectAlerted = false;
}

function recoverFail() {
    if (!disconnectAlerted) {
        alert("Warning: Lost worker connection!\n Input operations will not be delivered until re-connected.")
        disconnectAlerted = true;
    }
}
socket = undefined;
unload = false;
$(window).on('beforeunload', function(event) {
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
    if (element.hasOwnProperty('Job')) {
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
    unload = true;

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
                        data.forEach(function(element) {
                            handleRemoteOperation(element);
                        });
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
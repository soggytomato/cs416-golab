// Globals
debugMode = true;
recovering = false;
allowExecute = true;

workerIP = '';
userID = '';
sessionID = '';
currentSessions = [];
currentUsers = [];
jobIDs = new Map();
recoverLog = "";

/******************************* EVENT HANDLERS *******************************/

$(document).ready(function() {
    $('.input-wrapper').resizable({
        handles: 's',
        resize: function() {
            var curH = $(this).outerHeight();
            var curW = $(this).width();
            var containerH = $('.left').outerHeight();

            $('.input').height(curH - 30);
            editor.setSize(curW, curH - 30);

            $('.output-wrapper').height(containerH - curH);
            $('.output').height(containerH - curH - $('.output-wrapper').find('span').height());
        }
    });
});

$(document).ready(function() {
    getSessionAndUsernames();
    formBindings();
});

/******************************* SESSION & MORE *******************************/

function initSession() {
    $.ajax({
        type: 'get',
        url: 'http://' + workerIP + '/session',
        data: {
            sessionID: sessionID
        },
        success: function(data) {
            // CRDT Init
            const crdt = data.SessionRecord.CRDT;

            const ids = Object.keys(data.SessionRecord.CRDT);
            ids.forEach(function(id) {
                const element = crdt[id];
                const prev = element.PrevID == "" ? undefined : element.PrevID;
                const next = element.NextID == "" ? undefined : element.NextID;
                const val = element.Text;
                const del = element.Deleted;

                CRDT.seq[id] = new Element(id, prev, next, val, del)
            });

            CRDT.head = CRDT.getHead();
            CRDT.length = ids.length;

            mapping = CRDT.toMapping();

            editor.setValue(CRDT.toSnippet());

            // Log Records Init 
            const logs = data.LogRecord
            if (logs != null) {
                for (var i = 0; i < logs.length; i++) {
                    jobIDs.set(logs[i].Job.JobID, logs[i].Job.Done);
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
    })
}

function closeSession() {
    $.ajax({
        type: 'post',
        url: 'http://' + workerIP + '/session?userID=' + userID + '&sessionID=' + sessionID
    });

    if (socket.readyState == 0 || socket.readyState == 1) socket.close();
}

function getSessionAndUsernames() {
    $.ajax({
        type: 'get',
        url: '/sessions',
        success: function(data) {
            var sessSelect = document.getElementById("sessionSelect");
            for (var i = 0; i < data.ExistingSessions.length; i++) {
                if (!currentSessions.includes(data.ExistingSessions[i])) {
                    var opt = data.ExistingSessions[i];
                    var el = document.createElement("option");
                    el.textContent = opt;
                    el.value = opt;
                    sessSelect.appendChild(el);
                    currentSessions.push(opt);
                }
            }
            var userSelect = document.getElementById("userSelect");
            for (var i = 0; i < data.AllUsernames.length; i++) {
                if (!currentUsers.includes(data.AllUsernames[i])) {
                    var opt = data.AllUsernames[i];
                    var el = document.createElement("option");
                    el.textContent = opt;
                    el.value = opt;
                    userSelect.appendChild(el);
                    currentUsers.push(opt);
                }
            }
        }
    }).then(function() {
        setTimeout(getSessionAndUsernames, 2000);
    })
}

/******************************* REGISTRATION *******************************/

function formBindings() {
    $('#newUserRadio').on('click', function() {
        $('.new-user-group').css('display', 'block');
        $('.select-user-group').css('display', 'none');
    });

    $('#existingUserRadio').on('click', function() {
        $('.new-user-group').css('display', 'none');
        $('.select-user-group').css('display', 'block');
    });

    $('#newSessionRadio').on('click', function() {
        $('.new-session-group').css('display', 'block');
        $('.select-session-group').css('display', 'none');
    });

    $('#existingSessionRadio').on('click', function() {
        $('.new-session-group').css('display', 'none');
        $('.select-session-group').css('display', 'block');
    });

    $('#register').submit(function(e) {
        e.preventDefault();

        if (verifyRegister()) register();

        return false;
    });
};



function verifyRegister() {
    var valid = true;

    if ($('#existingUserRadio').is(':checked')) {
        if ($('#userSelect').find(':selected').attr('placeholder') === "") {
            alert("Please pick a user name.");
            valid = false;
        } else {
            userID = $('#userSelect').find(':selected').text();
        }
    } else {
        if ($('#userInput').val() == "") {
            alert("Please enter a user name.");
            valid = false;
        } else if ($('#userInput').val().indexOf(' ') >= 0) {
            alert("User name cannot contain whitespace.");
            valid = false;
        } else {
            userID = $('#userInput').val();
        }
    }

    if (valid == false) {
        return
    }

    if ($('#existingSessionRadio').is(':checked')) {
        if ($('#sessionSelect').find(':selected').attr('placeholder') === "") {
            alert("Please pick a session name.");
            valid = false;
        } else {
            sessionID = $('#sessionSelect').find(':selected').text();
        }
    } else {
        if ($('#sessionInput').val() == "") {
            alert("Please enter a session ID.");
            valid = false;
        } else if ($('#sessionInput').val().indexOf(' ') >= 0) {
            alert("Session ID cannot contain whitespace.");
            valid = false;
        } else {
            sessionID = $('#sessionInput').val();
            if (currentSessions.includes(sessionID)) {
                alert("Session ID is already taken, please enter a unique Session ID")
                valid = false
            }
        }
    }

    return valid;
}

function openEditor() {
    $('.register-wrapper').css('display', 'none');
    $('.editor').slideDown('slow');

    setTimeout(function() {
        editor.refresh()
    }, 500);
}

/******************************* EXECUTION & LOGS *******************************/

function reset() {
    $('.log-selected').removeClass('log-selected');

    editor.setValue(CRDT.toSnippet());

    $('#editArea').show();
    setTimeout(function() {
        editor.refresh()
    }, 500);
    $('#readOnlyArea').hide();

    document.getElementById('outputBox').innerHTML = "";
    document.getElementById("snipTitle").style.color = '';
    document.getElementById('snipTitle').innerHTML = "Snippet:";
}

function execute() {
    if (!allowExecute) {
        alert('No edits observed since last execution! See last log.');

        return;
    } else {
        allowExecute = false;
    }

    var newForm = document.createElement('form');
    newForm.setAttribute('id', 'executeForm');
    newForm.setAttribute('form', 'executeForm');

    var sessInput = document.createElement('input');
    sessInput.setAttribute('name', 'sessionID');
    sessInput.setAttribute('value', sessionID);
    sessInput.setAttribute('type', 'hidden');

    var snippet = document.createElement('textarea');
    snippet.setAttribute('name', 'snippet');
    snippet.value = CRDT.toSnippet();
    snippet.setAttribute('class', 'text');
    snippet.setAttribute('form', 'executeForm');

    newForm.append(sessInput);
    newForm.append(snippet);
    $("body").append(newForm);

    recoverLog = $('#executeForm').serialize();
    $.ajax({
        type: 'post',
        url: "http://" + workerIP + '/execute',
        dataType: 'json',
        data: $('#executeForm').serialize(),
        success: function(data) {
            jobIDs.set(data.JobID, false);
            //jobIDs.push(data.JobID);
            $("#logList").prepend("<li><a href=# id=" + data.JobID + ">" + data.JobID + "</a></li>")
            recoverLog = "";
        }
    })
    newForm.parentNode.removeChild(newForm);
}

function matchLog(log) {
    if (log.Job.SessionID == sessionID) {
        var isExist = false;
        for (var [key, value] of jobIDs) {
            if (key == log.Job.JobID) {
                jobIDs.set(log.Job.JobID, true);
                isExist = true;
                var logOutput = document.getElementById(log.Job.JobID);
                logOutput.addEventListener('click', function(e) {
                    e.preventDefault();
                    logClicked(log);
                }, false);
            }
        }
        if (!isExist) {
            jobIDs.set(log.Job.JobID, log.Job.Done);
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
    if ($('#' + log.Job.JobID).hasClass('log-selected')) {
        reset();

        return;
    }

    $('.log-selected').removeClass('log-selected');
    $('#' + log.Job.JobID).addClass('log-selected');

    editor_readOnly.setValue(log.Job.Snippet);
    editor_readOnly.setOption("readOnly", true);
    setTimeout(function() {
        editor_readOnly.refresh()
    }, 500);

    $('#editArea').hide();
    $('#readOnlyArea').show();

    str = log.Output.replace(/(?:\r\n|\r|\n)/g, '<br />');
    document.getElementById('outputBox').innerHTML = str;
    document.getElementById("snipTitle").style.color = '#dd7000';
    document.getElementById('snipTitle').innerHTML = "Snippet: READ ONLY";
}

/******************************* POPUP MESSAGES *******************************/

ERROR_MSG = undefined;
SUCCESS_MSG = undefined;

$(window).on('load', function() {
  const links = $('link[rel=import]');

    ERROR_MSG = $(links[0].import).find('.error-msg');
    SUCCESS_MSG = $(links[1].import).find('.success-msg');
});

function showMsg($msg, text, length) {
    $msg.find('.msg-text').text(text);
    $msg.prependTo('.msgs-wrapper');

    setTimeout(function(){
        const iconHeight = $msg.find('.msg-icon').height();
        const spanHeight = $msg.find('.msg-text').height();
        $msg.height(iconHeight + spanHeight); 
        $msg.css('display', 'none');
        $msg.css('visibility', 'visible');
        $msg.fadeIn('slow', function(){
            setTimeout(function(){
                $msg.fadeOut();
           }, length);
        });
    }, 250);
}

function showError(text, length) {
    const $msg = $(ERROR_MSG).clone();
    showMsg($msg, text, length);
}

function showSuccess(text, length) {
    const $msg = $(SUCCESS_MSG).clone();
    showMsg($msg, text, length);
}
$(document).ready(function(){
	editor = CodeMirror.fromTextArea(document.getElementById("code"), {
        theme: "dracula",
        matchBrackets: true,
        indentUnit: 8,
        tabSize: 8,
        indentWithTabs: true,
        mode: "text/x-go"
      });

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

$(document).ready(function(){
	formBindings();
});

function formBindings() {
	$('#newUser').on('click',function(){
		$('.new-user-group').css('display', 'block');
		$('.select-user-group').css('display', 'none');
	});

	$('#existingUser').on('click',function(){
		$('.new-user-group').css('display', 'none');
		$('.select-user-group').css('display', 'block');
	});

	$('#newSession').on('click',function(){
		$('.new-session-group').css('display', 'block');
		$('.select-session-group').css('display', 'none');
	});

	$('#existingSession').on('click',function(){
		$('.new-session-group').css('display', 'none');
		$('.select-session-group').css('display', 'block');
	});

	$('#register').submit(function (e) {
		e.preventDefault();

	 	$('.register').css('display', 'none');
		$('.editor').slideDown('slow');

		setTimeout(function(){
			editor.refresh();
		}, 500);

	 	return false;
	});
};
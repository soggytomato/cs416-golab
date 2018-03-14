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
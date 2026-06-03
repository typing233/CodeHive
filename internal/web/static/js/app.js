// CodeHive minimal JS
document.addEventListener('DOMContentLoaded', function() {
    // Auto-resize textareas
    document.querySelectorAll('textarea').forEach(function(ta) {
        ta.addEventListener('input', function() {
            this.style.height = 'auto';
            this.style.height = this.scrollHeight + 'px';
        });
    });

    // Confirm dangerous actions
    document.querySelectorAll('[data-confirm]').forEach(function(el) {
        el.addEventListener('click', function(e) {
            if (!confirm(this.dataset.confirm)) {
                e.preventDefault();
            }
        });
    });
});

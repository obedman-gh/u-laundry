try { localStorage.removeItem('token'); } catch (_) {}
const togglePassword = document.getElementById('togglePassword');
const passwordInput = document.getElementById('password');
const eyeOpen = document.getElementById('eyeOpen');
const eyeClosed = document.getElementById('eyeClosed');

togglePassword.addEventListener('click', function () {
    const type = passwordInput.getAttribute('type') === 'password' ? 'text' : 'password';
    passwordInput.setAttribute('type', type);
    this.setAttribute('aria-pressed', String(type === 'text'));
    this.setAttribute('aria-label', type === 'text' ? 'Sembunyikan kata sandi' : 'Tampilkan kata sandi');
    if (type === 'password') {
        eyeOpen.style.display = 'block';
        eyeClosed.style.display = 'none';
        this.setAttribute('title', 'Tampilkan kata sandi');
    } else {
        eyeOpen.style.display = 'none';
        eyeClosed.style.display = 'block';
        this.setAttribute('title', 'Sembunyikan kata sandi');
    }
});
document.getElementById('formLogin').addEventListener('submit', function(event) {
    event.preventDefault();

    const user = document.getElementById('username').value;
    const pass = document.getElementById('password').value;
    const btn = document.getElementById('btnLogin');
    const errorDiv = document.getElementById('pesan-error');

    if (btn.disabled) return;
    errorDiv.style.display = 'none';
    errorDiv.textContent = '';
    document.getElementById('formLogin').setAttribute('aria-busy','true');
    btn.textContent = 'Memverifikasi...';
    btn.disabled = true;

    fetch('/api/login', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({ username: user, password: pass })
    })
    .then(async response => {
        const data = await response.json().catch(() => ({}));
        
        if (response.ok && data.csrf_token) {
            window.location.href = '/index.html';
        } else {
            const message = data?.error?.message ?? data?.pesan;
            errorDiv.textContent = typeof message === 'string' ? message : 'Gagal login. Periksa kembali kredensial Anda.';
            errorDiv.style.display = 'block';
            btn.textContent = 'Masuk';
            btn.disabled = false;
            document.getElementById('formLogin').setAttribute('aria-busy','false');
            errorDiv.dataset.state = 'error';
        }
    })
    .catch(() => {
        errorDiv.textContent = 'Server tidak merespons. Periksa koneksi lalu coba kembali.';
        errorDiv.style.display = 'block';
        btn.textContent = 'Masuk';
        btn.disabled = false;
        document.getElementById('formLogin').setAttribute('aria-busy','false');
        errorDiv.dataset.state = 'error';
    });
});

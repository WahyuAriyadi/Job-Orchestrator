const { createApp } = Vue;

createApp({
  data() {
    return {
      apiBase: 'http://localhost:8080',
      jobs: [],
      health: null,
      healthOk: false,
      selectedJobId: null,
      executions: [],
      formMode: null, // null | 'create' | 'edit'
      formError: '',
      saving: false,
      form: this.emptyForm(),
      pollTimer: null,
    };
  },
  computed: {
    selectedJob() {
      return this.jobs.find(j => j.id === this.selectedJobId) || null;
    },
    healthDotClass() {
      if (!this.healthOk) return 'down';
      return this.health && this.health.is_leader ? 'leader' : 'standby';
    },
    healthLabel() {
      if (!this.healthOk) return 'unreachable';
      return this.health && this.health.is_leader ? 'leader (active)' : 'standby (follower)';
    },
    cronHint() {
      const map = {
        '* * * * *': 'setiap menit',
        '*/5 * * * *': 'setiap 5 menit',
        '*/15 * * * *': 'setiap 15 menit',
        '0 * * * *': 'setiap jam, menit ke-0',
        '0 0 * * *': 'setiap hari jam 00:00',
        '0 9 * * *': 'setiap hari jam 09:00',
        '0 9 * * 1-5': 'jam 09:00, Senin–Jumat',
        '0 0 1 * *': 'tanggal 1 tiap bulan, jam 00:00',
      };
      return map[this.form.cron_expression?.trim()] || ' ';
    },
  },
  methods: {
    emptyForm() {
      return {
        id: null, name: '', cron_expression: '*/15 * * * *', http_method: 'POST',
        callback_url: '', payloadText: '', max_retries: 3, timeout_seconds: 30, enabled: true,
      };
    },
    api(path) { return this.apiBase.replace(/\/$/, '') + path; },

    async fetchAll() {
      await Promise.all([this.fetchJobs(), this.fetchHealth()]);
    },
    async fetchHealth() {
      try {
        const res = await fetch(this.api('/api/health'));
        if (!res.ok) throw new Error();
        this.health = await res.json();
        this.healthOk = true;
      } catch (e) {
        this.healthOk = false;
      }
    },
    async fetchJobs() {
      try {
        const res = await fetch(this.api('/api/jobs'));
        this.jobs = await res.json();
      } catch (e) { /* backend unreachable; health pill already reflects this */ }
    },
    async selectJob(job) {
      this.formMode = null;
      this.selectedJobId = job.id;
      await this.refreshExecutions();
    },
    async refreshExecutions() {
      if (!this.selectedJobId) return;
      try {
        const res = await fetch(this.api(`/api/jobs/${this.selectedJobId}/executions`));
        this.executions = await res.json();
      } catch (e) { this.executions = []; }
    },
    async triggerJob(job) {
      await fetch(this.api(`/api/jobs/${job.id}/trigger`), { method: 'POST' });
      if (this.selectedJobId === job.id) setTimeout(() => this.refreshExecutions(), 800);
    },
    async deleteJob(job) {
      if (!confirm(`Hapus job "${job.name}"? Tindakan ini tidak bisa dibatalkan.`)) return;
      await fetch(this.api(`/api/jobs/${job.id}`), { method: 'DELETE' });
      if (this.selectedJobId === job.id) { this.selectedJobId = null; this.executions = []; }
      await this.fetchJobs();
    },

    openCreateForm() {
      this.form = this.emptyForm();
      this.formError = '';
      this.formMode = 'create';
    },
    openEditForm(job) {
      this.form = {
        id: job.id, name: job.name, cron_expression: job.cron_expression,
        http_method: job.http_method, callback_url: job.callback_url,
        payloadText: job.payload ? JSON.stringify(job.payload, null, 2) : '',
        max_retries: job.max_retries, timeout_seconds: job.timeout_seconds, enabled: job.enabled,
      };
      this.formError = '';
      this.formMode = 'edit';
    },
    closeForm() { this.formMode = null; this.formError = ''; },

    async submitForm() {
      this.formError = '';
      if (!this.form.name || !this.form.cron_expression || !this.form.callback_url) {
        this.formError = 'name, cron expression, dan callback url wajib diisi.';
        return;
      }
      let payload = undefined;
      if (this.form.payloadText.trim()) {
        try { payload = JSON.parse(this.form.payloadText); }
        catch (e) { this.formError = 'payload bukan JSON valid.'; return; }
      }

      const body = {
        name: this.form.name,
        cron_expression: this.form.cron_expression,
        http_method: this.form.http_method,
        callback_url: this.form.callback_url,
        payload: payload,
        max_retries: this.form.max_retries,
        timeout_seconds: this.form.timeout_seconds,
        enabled: this.form.enabled,
      };

      this.saving = true;
      try {
        const isEdit = this.formMode === 'edit';
        const url = isEdit ? this.api(`/api/jobs/${this.form.id}`) : this.api('/api/jobs');
        const res = await fetch(url, {
          method: isEdit ? 'PUT' : 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        });
        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          throw new Error(err.error || `HTTP ${res.status}`);
        }
        await this.fetchJobs();
        this.formMode = null;
      } catch (e) {
        this.formError = e.message;
      } finally {
        this.saving = false;
      }
    },

    shortUrl(u) {
      try { const url = new URL(u); return url.hostname + url.pathname; }
      catch (e) { return u; }
    },
    formatTime(iso) {
      if (!iso) return '—';
      const d = new Date(iso);
      return d.toLocaleString('id-ID', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });
    },
  },
  mounted() {
    this.fetchAll();
    this.pollTimer = setInterval(() => {
      this.fetchAll();
      if (this.selectedJobId && !this.formMode) this.refreshExecutions();
    }, 5000);
  },
  beforeUnmount() { clearInterval(this.pollTimer); },
}).mount('#app');

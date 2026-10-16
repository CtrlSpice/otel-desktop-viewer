// Plain JavaScript embedded after the renderer's delegated metric controls.
export const liveClient = String.raw`(() => {
  const main = document.querySelector('main');
  const states = {
    waiting: 'Waiting for saved results',
    running: 'Running · saved results',
    stopped: 'Stopped · saved results',
    complete: 'Complete · saved results',
    unavailable: 'Saved results unavailable'
  };
  const views = ['result', 'time', 'cost'];
  const controls = 'summary,button,a[href],input,select,textarea,[tabindex]';
  let recorded = null;
  let connected = false;
  let invalid = false;
  let lastHtml = null;

  function showStatus() {
    const status = main.querySelector('#live-status');
    if (!status) return;
    const state = recorded ? states[recorded.state] : 'Waiting for saved results';
    const time = recorded ? ' · Updated ' + new Date(recorded.updatedAt).toLocaleString() : '';
    const health = invalid ? ' · Updates unavailable' : connected ? '' : ' · Reconnecting';
    status.textContent = state + time + health;
  }

  function modelRows() {
    return Array.from(main.querySelectorAll('.model[data-live-key]'));
  }

  function rememberFocus() {
    const active = document.activeElement;
    if (!main.contains(active)) return null;
    if (active.matches('[data-view]')) return { metric: active.dataset.view };
    if (active.id) return { id: active.id };
    const model = active.closest('.model[data-live-key]');
    return model ? {
      key: model.dataset.liveKey,
      index: Array.from(model.querySelectorAll(controls)).indexOf(active)
    } : null;
  }

  function restoreFocus(saved, rows) {
    if (!saved) return;
    let target;
    if (saved.metric) {
      target = Array.from(main.querySelectorAll('[data-view]')).find(item => item.dataset.view === saved.metric);
    } else if (saved.id) {
      target = document.getElementById(saved.id);
    } else {
      const model = rows.find(item => item.dataset.liveKey === saved.key);
      target = model && model.querySelectorAll(controls)[saved.index];
    }
    if (target && typeof target.focus === 'function') target.focus({ preventScroll: true });
  }

  function replaceResults(html) {
    if (html === lastHtml) return;
    const template = document.createElement('template');
    template.innerHTML = html;
    // Snapshot content is renderer output, never an additional script source.
    template.content.querySelectorAll('script').forEach(script => script.remove());
    const metric = document.body.dataset.metric;
    const focus = rememberFocus();
    const x = window.scrollX, y = window.scrollY;
    const expandedTables = new Set(Array.from(main.querySelectorAll('[data-grading-table][aria-expanded="true"]')).map(button => button.dataset.gradingTable));
    const saved = new Map(modelRows().map(model => [model.dataset.liveKey, {
      open: model instanceof HTMLDetailsElement ? model.open : null,
      horizontal: new Map(Array.from(model.querySelectorAll('.grading-output,.grader-metrics-scroll')).map((output, index) => [output.dataset.gradingScrollKey || index, output.scrollLeft]))
    }]));
    main.replaceChildren(template.content);
    const rows = modelRows();
    for (const model of rows) {
      const previous = saved.get(model.dataset.liveKey);
      if (!previous) continue;
      if (model instanceof HTMLDetailsElement && previous.open !== null) model.open = previous.open;
      model.querySelectorAll('.grading-output,.grader-metrics-scroll').forEach((output, index) => {
        output.scrollLeft = previous.horizontal.get(output.dataset.gradingScrollKey || index) || 0;
      });
    }
    main.querySelectorAll('[data-grading-table]').forEach(button => {
      const body = document.getElementById(button.dataset.gradingTable);
      if (body && expandedTables.has(button.dataset.gradingTable)) {
        button.setAttribute('aria-expanded', 'true');
        body.hidden = false;
      }
    });
    if (views.includes(metric)) {
      document.body.dataset.metric = metric;
      main.querySelectorAll('[data-view]').forEach(button => {
        const selected = button.dataset.view === metric;
        button.setAttribute('aria-pressed', String(selected));
      });
    }
    restoreFocus(focus, rows);
    if (window.scrollX !== x || window.scrollY !== y) window.scrollTo(x, y);
    lastHtml = html;
  }

  const events = new EventSource('/events');
  events.onopen = () => { connected = true; showStatus(); };
  events.onerror = () => { connected = false; showStatus(); };
  events.addEventListener('snapshot', event => {
    let snapshot;
    try {
      snapshot = JSON.parse(event.data);
      if (!snapshot || Array.isArray(snapshot) || typeof snapshot !== 'object' ||
          Object.keys(snapshot).some(key => !['html', 'state', 'updatedAt'].includes(key)) ||
          typeof snapshot.html !== 'string' ||
          typeof snapshot.state !== 'string' || !Object.hasOwn(states, snapshot.state) ||
          typeof snapshot.updatedAt !== 'string' || !Number.isFinite(Date.parse(snapshot.updatedAt))) {
        throw new Error('Invalid snapshot');
      }
    } catch {
      invalid = true;
      showStatus();
      return;
    }
    replaceResults(snapshot.html);
    recorded = { state: snapshot.state, updatedAt: snapshot.updatedAt };
    invalid = false;
    showStatus();
  });
  showStatus();
})();`

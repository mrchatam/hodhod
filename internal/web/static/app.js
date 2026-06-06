(function () {
  'use strict';

  /* Theme flash (before paint) */
  (function themeFlash() {
    var t = document.documentElement.getAttribute('data-theme');
    if (t === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      document.documentElement.classList.add('dark');
    } else if (t === 'system') {
      document.documentElement.classList.remove('dark');
    }
  })();

  function $(sel, root) {
    return (root || document).querySelector(sel);
  }
  function $$(sel, root) {
    return Array.prototype.slice.call((root || document).querySelectorAll(sel));
  }

  /* Mobile bottom sheet */
  var sheet = $('#mobile-sheet');
  var backdrop = $('#sheet-backdrop');
  var navToggle = $('#nav-toggle');
  var bottomMore = $('#bottom-nav-more');

  function closeSheet() {
    if (sheet) {
      sheet.classList.remove('open');
      sheet.setAttribute('aria-hidden', 'true');
    }
    if (backdrop) backdrop.classList.add('hidden');
    document.body.style.overflow = '';
  }

  function openSheet() {
    if (sheet) {
      sheet.classList.add('open');
      sheet.setAttribute('aria-hidden', 'false');
    }
    if (backdrop) backdrop.classList.remove('hidden');
    document.body.style.overflow = 'hidden';
  }

  function toggleSheet() {
    if (sheet && sheet.classList.contains('open')) closeSheet();
    else openSheet();
  }

  if (navToggle) navToggle.addEventListener('click', toggleSheet);
  if (bottomMore) bottomMore.addEventListener('click', toggleSheet);
  if (backdrop) backdrop.addEventListener('click', closeSheet);

  $$('[data-sheet-close]').forEach(function (el) {
    el.addEventListener('click', closeSheet);
  });

  $$('#mobile-sheet .nav-link').forEach(function (link) {
    link.addEventListener('click', closeSheet);
  });

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') {
      closeSheet();
      $$('.modal-backdrop:not(.hidden)').forEach(function (m) {
        m.classList.add('hidden');
      });
    }
  });

  /* Modals */
  $$('.modal-backdrop[data-close]').forEach(function (el) {
    el.addEventListener('click', function (e) {
      if (e.target === el) el.classList.add('hidden');
    });
  });

  document.body.addEventListener('click', function (e) {
    var openBtn = e.target.closest('[data-modal-open]');
    if (openBtn) {
      var id = openBtn.getAttribute('data-modal-open');
      var modal = id && document.getElementById(id);
      if (modal) modal.classList.remove('hidden');
      return;
    }
    var closeBtn = e.target.closest('[data-modal-close]');
    if (closeBtn) {
      var closeId = closeBtn.getAttribute('data-modal-close');
      var closeModal = closeId ? document.getElementById(closeId) : closeBtn.closest('.modal-backdrop');
      if (closeModal) closeModal.classList.add('hidden');
    }
  });

  /* Form busy states */
  document.body.addEventListener('submit', function (e) {
    var form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (form.dataset.noBusy === '1') return;
    var btn = form.querySelector('button[type="submit"]');
    if (!btn || btn.getAttribute('aria-busy') === 'true') return;
    btn.setAttribute('aria-busy', 'true');
    var label = btn.querySelector('.btn-label');
    if (label && btn.dataset.busyText) label.textContent = btn.dataset.busyText;
  });

  document.body.addEventListener('htmx:beforeRequest', function (e) {
    var elt = e.detail.elt;
    if (!elt) return;
    var btn = elt.tagName === 'BUTTON' ? elt : elt.querySelector('button[type="submit"]');
    if (btn) btn.setAttribute('aria-busy', 'true');
  });

  document.body.addEventListener('htmx:afterRequest', function (e) {
    var elt = e.detail.elt;
    if (!elt) return;
    var btn = elt.tagName === 'BUTTON' ? elt : elt.querySelector('button[type="submit"]');
    if (btn) btn.removeAttribute('aria-busy');
  });

  /* Action menus */
  $$('[data-action-menu]').forEach(function (wrap) {
    var btn = wrap.querySelector('[data-action-menu-btn]');
    var panel = wrap.querySelector('[data-action-menu-panel]');
    if (!btn || !panel) return;
    btn.addEventListener('click', function (e) {
      e.stopPropagation();
      panel.classList.toggle('hidden');
    });
    document.addEventListener('click', function () {
      panel.classList.add('hidden');
    });
  });

  /* Tabs with hash routing */
  function activateTab(tabId) {
    var tabs = $$('[data-tab]');
    var panels = $$('.tab-panel');
    if (!tabs.length) return;
    tabs.forEach(function (t) {
      t.classList.toggle('tab-active', t.getAttribute('data-tab') === tabId);
    });
    panels.forEach(function (p) {
      p.classList.toggle('hidden', p.id !== 'tab-' + tabId);
    });
  }

  $$('[data-tab]').forEach(function (tab) {
    tab.addEventListener('click', function () {
      var id = tab.getAttribute('data-tab');
      activateTab(id);
      if (id && history.replaceState) {
        history.replaceState(null, '', '#tab-' + id);
      }
    });
  });

  (function initTabFromHash() {
    var hash = location.hash.replace(/^#tab-/, '');
    if (hash && document.getElementById('tab-' + hash)) {
      activateTab(hash);
    }
  })();

  /* Copy to clipboard */
  document.body.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-copy]');
    if (!btn) return;
    var text = btn.getAttribute('data-copy');
    if (text && navigator.clipboard) navigator.clipboard.writeText(text);
  });

  /* Navigate on card click */
  document.body.addEventListener('click', function (e) {
    var card = e.target.closest('[data-href]');
    if (!card || e.target.closest('a, button, input, select, textarea, form, [data-action-menu]')) return;
    var href = card.getAttribute('data-href');
    if (href) location.href = href;
  });

  /* Toggle row drawer */
  document.body.addEventListener('click', function (e) {
    var row = e.target.closest('[data-row-toggle]');
    if (!row || e.target.closest('[data-stop-toggle]')) return;
    var targetId = row.getAttribute('data-row-toggle');
    var drawer = targetId && document.getElementById(targetId);
    if (drawer) drawer.classList.toggle('hidden');
  });

  /* Panel type fields (panels page) */
  (function panelTypeSync() {
    var sel = $('#panel-type');
    if (!sel) return;
    var hint = $('#hint-base-url');
    function sync() {
      var xui = sel.value === 'xui';
      $$('.xui-field').forEach(function (el) {
        el.style.display = xui ? '' : 'none';
      });
      if (hint) {
        hint.textContent = xui ? (sel.getAttribute('data-hint-xui') || '') : (sel.getAttribute('data-hint-marzban') || '');
      }
    }
    sel.addEventListener('change', sync);
    sync();
  })();

  /* QR modal (services page) */
  $$('.qr-btn').forEach(function (btn) {
    btn.addEventListener('click', function () {
      if (typeof QRCode === 'undefined') return;
      var canvas = $('#qr-canvas');
      var modal = $('#qr-modal');
      if (!canvas || !modal) return;
      QRCode.toCanvas(canvas, btn.dataset.sub, { width: 200 });
      modal.classList.remove('hidden');
    });
  });

  /* Panel user template picker */
  function applyTemplate(btn) {
    var vol = $('#create-volume');
    var days = $('#create-days');
    var note = $('#create-note');
    var ip = $('#create-ip');
    var tplName = $('#create-template-name');
    if (vol) vol.value = btn.dataset.volume || 30;
    if (days) days.value = btn.dataset.days || 30;
    if (note) note.value = btn.dataset.note || '';
    if (ip) ip.value = btn.dataset.ip || 0;
    if (tplName) tplName.value = btn.dataset.name || '';
    var ids = (btn.dataset.inbounds || '').split(',').filter(Boolean);
    $$('.inbound-cb').forEach(function (cb) {
      if (cb.type === 'radio') {
        cb.checked = ids.indexOf(cb.value) !== -1;
      } else {
        cb.checked = ids.indexOf(cb.value) !== -1;
      }
    });
    var modal = $('#master-create-modal');
    if (modal) modal.classList.remove('hidden');
  }

  $$('.template-pick').forEach(function (btn) {
    btn.addEventListener('click', function () {
      applyTemplate(btn);
    });
  });

  $$('.template-edit').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var name = $('#tpl-name');
      var vol = $('#tpl-volume');
      var days = $('#tpl-days');
      var ip = $('#tpl-ip');
      var note = $('#tpl-note');
      if (name) name.value = btn.dataset.name || '';
      if (vol) vol.value = btn.dataset.volume || 30;
      if (days) days.value = btn.dataset.days || 30;
      if (ip) ip.value = btn.dataset.ip || 0;
      if (note) note.value = btn.dataset.note || '';
      var ids = (btn.dataset.inbounds || '').split(',').filter(Boolean);
      $$('.tpl-inbound-cb').forEach(function (cb) {
        cb.checked = ids.indexOf(cb.value) !== -1;
      });
      var form = $('#template-save-form');
      if (form) form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  });

  window.syncTemplateForm = function () {
    var tplVol = $('#tpl-volume');
    var tplDays = $('#tpl-days');
    var tplIp = $('#tpl-ip');
    var tplNote = $('#tpl-note');
    var vol = $('#create-volume');
    var days = $('#create-days');
    var note = $('#create-note');
    var ip = $('#create-ip');
    if (tplVol && vol) tplVol.value = vol.value;
    if (tplDays && days) tplDays.value = days.value;
    if (tplNote && note) tplNote.value = note.value;
    if (tplIp && ip) tplIp.value = ip.value;
    var ids = [];
    $$('.inbound-cb:checked').forEach(function (cb) {
      ids.push(cb.value);
    });
    $$('.tpl-inbound-cb').forEach(function (cb) {
      cb.checked = ids.indexOf(cb.value) !== -1;
    });
  };

  /* Agent panel assign: show inbounds for selected panel */
  (function agentPanelAssignSync() {
    var sel = $('#agent-assign-panel');
    if (!sel) return;
    function sync() {
      var pid = sel.value;
      $$('.agent-assign-inbounds').forEach(function (el) {
        el.classList.toggle('hidden', el.getAttribute('data-panel-id') !== pid);
      });
    }
    sel.addEventListener('change', sync);
    sync();
  })();

  /* Bot panel assign: show inbounds for selected panel */
  (function botPanelAssignSync() {
    var sel = $('#bot-assign-panel');
    if (!sel) return;
    function sync() {
      var pid = sel.value;
      $$('.bot-panel-inbounds').forEach(function (el) {
        el.classList.toggle('hidden', el.getAttribute('data-panel-id') !== pid);
      });
    }
    sel.addEventListener('change', sync);
    sync();
  })();

  /* Depleted clients confirm */
  document.body.addEventListener('submit', function (e) {
    var form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (form.dataset.confirm && !window.confirm(form.dataset.confirm)) {
      e.preventDefault();
    }
  });
})();

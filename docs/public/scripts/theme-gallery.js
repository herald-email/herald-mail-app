(() => {
  if (window.__heraldThemeGalleryZoom) {
    return;
  }
  window.__heraldThemeGalleryZoom = true;

  const triggerSelector = '.theme-shot-grid a[href$=".png"]';
  let overlay;
  let overlayImage;
  let overlayCaption;
  let lastTrigger;

  function ensureOverlay() {
    if (overlay) {
      return;
    }

    overlay = document.createElement('div');
    overlay.className = 'theme-zoom';
    overlay.hidden = true;
    overlay.tabIndex = -1;
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-label', 'Expanded theme screenshot');

    const frame = document.createElement('div');
    frame.className = 'theme-zoom__frame';

    overlayImage = document.createElement('img');
    overlayImage.className = 'theme-zoom__image';
    overlayImage.alt = '';

    overlayCaption = document.createElement('div');
    overlayCaption.className = 'theme-zoom__caption';

    frame.append(overlayImage, overlayCaption);
    overlay.append(frame);
    document.body.append(overlay);

    overlay.addEventListener('click', closeZoom);
  }

  function openZoom(trigger) {
    ensureOverlay();

    const image = trigger.querySelector('img');
    const href = trigger.getAttribute('href');
    lastTrigger = trigger;
    overlayImage.src = new URL(href, window.location.href).href;
    overlayImage.alt = image?.alt || 'Expanded theme screenshot';
    overlayCaption.textContent = image?.alt || href;
    overlay.hidden = false;
    document.body.classList.add('theme-zoom-open');
    overlay.focus({ preventScroll: true });
  }

  function closeZoom() {
    if (!overlay || overlay.hidden) {
      return;
    }

    overlay.hidden = true;
    overlayImage.removeAttribute('src');
    document.body.classList.remove('theme-zoom-open');
    lastTrigger?.focus({ preventScroll: true });
  }

  document.addEventListener('click', (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }

    const trigger = event.target.closest(triggerSelector);
    if (!trigger) {
      return;
    }

    event.preventDefault();
    openZoom(trigger);
  });

  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
      closeZoom();
    }
  });
})();

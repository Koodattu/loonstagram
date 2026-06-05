const form = document.querySelector("#convert-form");
const sourceInput = document.querySelector("#instagram-url");
const submitButton = document.querySelector("#submit-button");
const fixAnotherButton = document.querySelector("#fix-another-button");
const statusText = document.querySelector("#status");

const galleryProfile = document.querySelector("#gallery-profile");
const galleryGrid = document.querySelector("#gallery-grid");
const galleryStatus = document.querySelector("#gallery-status");
const galleryRefresh = document.querySelector("#gallery-refresh");
const viewer = document.querySelector("#gallery-viewer");
const viewerClose = document.querySelector("#viewer-close");
const viewerPrev = document.querySelector("#viewer-prev");
const viewerNext = document.querySelector("#viewer-next");
const viewerMedia = document.querySelector("#viewer-media");
const viewerDots = document.querySelector("#viewer-dots");
const viewerMediaPrev = document.querySelector("#viewer-media-prev");
const viewerMediaNext = document.querySelector("#viewer-media-next");
const viewerTitle = document.querySelector("#viewer-title");
const viewerCaption = document.querySelector("#viewer-caption");
const viewerFixed = document.querySelector("#viewer-fixed");
const viewerOriginal = document.querySelector("#viewer-original");

const adminForm = document.querySelector("#admin-form");
const adminDialog = document.querySelector("#admin-dialog");
const adminInput = document.querySelector("#admin-token");
const adminButton = document.querySelector("#admin-button");
const automationPanel = document.querySelector("#automation-panel");
const automationStatusText = document.querySelector("#automation-status");
const adminDialogStatusText = document.querySelector("#admin-dialog-status");
const instagramForm = document.querySelector("#instagram-form");
const instagramInput = document.querySelector("#instagram-username");
const instagramCurrent = document.querySelector("#instagram-current");
const automationEnabled = document.querySelector("#automation-enabled");
const instagramSave = document.querySelector("#instagram-save");
const discordForm = document.querySelector("#discord-form");
const discordWebhook = document.querySelector("#discord-webhook");
const discordSave = document.querySelector("#discord-save");
const discordCurrent = document.querySelector("#discord-current");
const discordOAuth = document.querySelector("#discord-oauth");
const discordDisconnect = document.querySelector("#discord-disconnect");
const discordTest = document.querySelector("#discord-test");
const lastCheck = document.querySelector("#last-check");
const lastPost = document.querySelector("#last-post");
const automationResult = document.querySelector("#automation-result");

const adminStorageKey = "Loonstagram.adminToken";
const savedAdminToken = localStorage.getItem(adminStorageKey);

const galleryState = {
  items: [],
  tiles: [],
  postIndex: 0,
  mediaIndex: 0,
  viewerDirection: "",
};

let fixedURLMode = false;
let statusAnimationTimer = 0;
let viewerWheelTimer = 0;

function setStatus(message, kind = "") {
  if (!statusText) {
    return;
  }
  statusText.textContent = message;
  statusText.classList.remove("is-updating");
  if (kind) {
    statusText.dataset.kind = kind;
  } else {
    delete statusText.dataset.kind;
  }
  if (message) {
    window.requestAnimationFrame(() => {
      statusText.classList.add("is-updating");
      window.clearTimeout(statusAnimationTimer);
      statusAnimationTimer = window.setTimeout(() => {
        statusText.classList.remove("is-updating");
      }, 180);
    });
  }
}

function setAutomationStatus(message, kind = "") {
  const targets = [automationStatusText, adminDialogStatusText].filter(Boolean);
  if (targets.length === 0) {
    return;
  }
  targets.forEach((target) => {
    target.textContent = message;
    if (kind) {
      target.dataset.kind = kind;
    } else {
      delete target.dataset.kind;
    }
  });
}

function setGalleryStatus(message, kind = "") {
  if (!galleryStatus) {
    return;
  }
  galleryStatus.textContent = message;
  if (kind) {
    galleryStatus.dataset.kind = kind;
  } else {
    delete galleryStatus.dataset.kind;
  }
}

function adminToken() {
  if (!adminInput) {
    return "";
  }
  return adminInput.value.trim();
}

function adminHeaders() {
  return {
    "Content-Type": "application/json",
    "X-Admin-Token": adminToken(),
  };
}

async function readJSON(response) {
  try {
    return await response.json();
  } catch {
    return {};
  }
}

function formatTime(value) {
  if (!value) {
    return "Never";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "Never";
  }
  return date.toLocaleString([], {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

function updateAutomationUI(payload) {
  if (!automationPanel) {
    return;
  }
  automationPanel.hidden = false;
  if (adminDialog && adminDialog.open) {
    adminDialog.close();
  }
  instagramInput.value = payload.instagramUsername || "";
  automationEnabled.checked = Boolean(payload.enabled);
  instagramCurrent.textContent = payload.instagramUsername ? `@${payload.instagramUsername}` : "Not configured";

  discordCurrent.textContent = payload.discordConnected
    ? payload.discordLabel || "Connected"
    : "Not connected";
  discordDisconnect.hidden = !payload.discordConnected;
  discordOAuth.hidden = !payload.discordOAuthConfigured;

  lastCheck.textContent = formatTime(payload.lastCheckedAt);
  lastPost.textContent = formatTime(payload.lastPostedAt);
  automationResult.textContent = payload.lastError || payload.lastStatus || "Idle";
}

async function loadAutomation() {
  if (!adminToken()) {
    setAutomationStatus("Enter admin password.", "error");
    return;
  }

  adminButton.disabled = true;
  setAutomationStatus("Loading settings...");
  try {
    const response = await fetch("/api/automation/status", {
      headers: {
        "X-Admin-Token": adminToken(),
      },
    });
    const payload = await readJSON(response);
    if (!response.ok || !payload.ok) {
      setAutomationStatus(payload.error || "Could not load automation settings.", "error");
      automationPanel.hidden = true;
      if (adminDialog && !adminDialog.open && typeof adminDialog.showModal === "function") {
        adminDialog.showModal();
      }
      return;
    }
    localStorage.setItem(adminStorageKey, adminToken());
    updateAutomationUI(payload);
    setAutomationStatus("Settings unlocked.", "success");
    if (galleryGrid) {
      loadGallery();
    }
  } catch {
    setAutomationStatus("Could not load automation settings.", "error");
    if (adminDialog && !adminDialog.open && typeof adminDialog.showModal === "function") {
      adminDialog.showModal();
    }
  } finally {
    adminButton.disabled = false;
  }
}

async function loadGallery() {
  if (!galleryGrid) {
    return;
  }
  if (galleryRefresh) {
    galleryRefresh.disabled = true;
  }
  galleryGrid.setAttribute("aria-busy", "true");
  renderGallerySkeleton();
  setGalleryStatus("");
  try {
    const response = await fetch("/api/gallery", {
      headers: {
        "Accept": "application/json",
      },
    });
    const payload = await readJSON(response);
    if (!response.ok || !payload.ok) {
      setGalleryStatus(payload.error || "Could not load gallery.", "error");
      renderGalleryEmpty(payload.error || "Could not load gallery.");
      return;
    }
    if (galleryProfile) {
      galleryProfile.textContent = payload.profile || "loonletwow";
    }
    galleryState.items = Array.isArray(payload.items) ? payload.items : [];
    galleryState.tiles = galleryTiles(galleryState.items);
    if (galleryState.tiles.length === 0) {
      renderGalleryEmpty(payload.empty || "No cached gallery posts yet.");
      setGalleryStatus("");
    } else {
      renderGallery();
      setGalleryStatus("");
    }
  } catch {
    renderGalleryEmpty("Gallery unavailable.");
    setGalleryStatus("Gallery unavailable.", "error");
  } finally {
    galleryGrid.removeAttribute("aria-busy");
    if (galleryRefresh) {
      galleryRefresh.disabled = false;
    }
  }
}

function renderGallerySkeleton(count = 9) {
  if (!galleryGrid) {
    return;
  }
  const skeletons = Array.from({ length: count }, (_, index) => {
    const skeleton = document.createElement("div");
    skeleton.className = "gallery-card is-skeleton";
    skeleton.setAttribute("aria-hidden", "true");
    skeleton.style.setProperty("--tile-index", String(Math.min(index, 8)));
    return skeleton;
  });
  galleryGrid.replaceChildren(...skeletons);
}

function renderGallery() {
  if (!galleryGrid) {
    return;
  }
  const cards = galleryState.tiles.map((tile, index) => {
    const button = document.createElement("button");
    button.className = "gallery-card";
    button.type = "button";
    button.setAttribute("aria-label", `Open post ${tile.post.shortcode}, image ${tile.mediaIndex + 1}`);
    button.style.setProperty("--tile-index", String(Math.min(index, 8)));
    button.addEventListener("click", () => openViewer(tile.postIndex, tile.mediaIndex));

    if (tile.imageURL) {
      const image = document.createElement("img");
      image.src = tile.imageURL;
      image.alt = tile.post.caption ? `@${tile.post.username}: ${tile.post.caption}` : `@${tile.post.username} Instagram post`;
      image.loading = "lazy";
      button.append(image);
    }

    if (tile.media.videoUrl) {
      const badge = document.createElement("span");
      badge.className = "gallery-badge";
      badge.textContent = "Video";
      button.append(badge);
    }
    return button;
  });
  galleryGrid.replaceChildren(...cards);
}

function renderGalleryEmpty(message) {
  if (!galleryGrid) {
    return;
  }
  const empty = document.createElement("div");
  empty.className = "gallery-empty";
  empty.setAttribute("role", "note");

  const text = document.createElement("p");
  text.textContent = message;
  empty.append(text);
  galleryGrid.replaceChildren(empty);
}

function galleryTiles(items) {
  const out = [];
  items.forEach((post, postIndex) => {
    if (!Array.isArray(post.media)) {
      return;
    }
    post.media.forEach((media, mediaIndex) => {
      if (!media.imageUrl) {
        return;
      }
      out.push({
        post,
        postIndex,
        media,
        mediaIndex,
        imageURL: media.imageUrl,
      });
    });
  });
  return out;
}

function openViewer(postIndex, mediaIndex) {
  if (!viewer || !galleryState.items.length) {
    return;
  }
  galleryState.postIndex = clampIndex(postIndex, galleryState.items.length);
  const post = galleryState.items[galleryState.postIndex];
  galleryState.mediaIndex = clampIndex(mediaIndex, post.media.length);
  galleryState.viewerDirection = "";
  renderViewer();
  document.body.classList.add("viewer-open");
  if (typeof viewer.showModal === "function" && !viewer.open) {
    viewer.showModal();
  } else {
    viewer.setAttribute("open", "");
  }
}

function closeViewer() {
  if (!viewer) {
    return;
  }
  if (viewer.open && typeof viewer.close === "function") {
    viewer.close();
  } else {
    viewer.removeAttribute("open");
  }
  document.body.classList.remove("viewer-open");
}

function renderViewer() {
  if (!viewerMedia) {
    return;
  }
  const post = galleryState.items[galleryState.postIndex];
  if (!post) {
    return;
  }
  const media = post.media[galleryState.mediaIndex] || post.media[0];
  viewerTitle.textContent = post.username ? `@${post.username}` : "@loonletwow";
  viewerCaption.textContent = post.caption || "No caption cached.";
  viewerFixed.href = post.canonicalUrl || "#";
  viewerOriginal.href = post.originalUrl || "#";

  if (galleryState.viewerDirection) {
    viewerMedia.dataset.direction = galleryState.viewerDirection;
  } else {
    delete viewerMedia.dataset.direction;
  }
  viewerMedia.replaceChildren(renderViewerMedia(media, post));
  renderViewerDots(post);
  galleryState.viewerDirection = "";

  const hasMultiplePosts = galleryState.items.length > 1;
  viewerPrev.disabled = !hasMultiplePosts;
  viewerNext.disabled = !hasMultiplePosts;
  const hasMultipleMedia = post.media.length > 1;
  viewerMediaPrev.hidden = !hasMultipleMedia;
  viewerMediaNext.hidden = !hasMultipleMedia;
  viewerDots.hidden = !hasMultipleMedia;
}

function renderViewerMedia(media, post) {
  if (media && media.videoUrl) {
    const video = document.createElement("video");
    video.className = "viewer-media-item";
    video.src = media.videoUrl;
    video.poster = media.imageUrl || "";
    video.controls = true;
    video.playsInline = true;
    video.preload = "metadata";
    video.setAttribute("aria-label", `Video from ${post.shortcode}`);
    return video;
  }

  const image = document.createElement("img");
  image.className = "viewer-media-item";
  image.src = media && media.imageUrl ? media.imageUrl : "";
  image.alt = post.caption ? `@${post.username}: ${post.caption}` : `@${post.username} Instagram image`;
  return image;
}

function renderViewerDots(post) {
  const dots = post.media.map((_, index) => {
    const dot = document.createElement("button");
    dot.className = "viewer-dot";
    dot.type = "button";
    dot.setAttribute("aria-label", `View image ${index + 1}`);
    if (index === galleryState.mediaIndex) {
      dot.setAttribute("aria-current", "true");
    }
    dot.addEventListener("click", () => {
      galleryState.mediaIndex = index;
      renderViewer();
    });
    return dot;
  });
  viewerDots.replaceChildren(...dots);
}

function showRelativePost(offset) {
  if (!galleryState.items.length) {
    return;
  }
  galleryState.postIndex = wrapIndex(galleryState.postIndex + offset, galleryState.items.length);
  galleryState.mediaIndex = 0;
  galleryState.viewerDirection = offset > 0 ? "next" : "prev";
  renderViewer();
}

function showRelativeMedia(offset) {
  const post = galleryState.items[galleryState.postIndex];
  if (!post || post.media.length <= 1) {
    return;
  }
  galleryState.mediaIndex = wrapIndex(galleryState.mediaIndex + offset, post.media.length);
  galleryState.viewerDirection = offset > 0 ? "next" : "prev";
  renderViewer();
}

function handleViewerWheel(event) {
  if (!viewer || !viewer.open) {
    return;
  }
  const strongestDelta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
  if (Math.abs(strongestDelta) < 18 || viewerWheelTimer) {
    return;
  }
  event.preventDefault();
  viewerWheelTimer = window.setTimeout(() => {
    viewerWheelTimer = 0;
  }, 260);
  const direction = strongestDelta > 0 ? 1 : -1;
  const post = galleryState.items[galleryState.postIndex];
  if (post && post.media.length > 1) {
    showRelativeMedia(direction);
  } else {
    showRelativePost(direction);
  }
}

function wrapIndex(index, length) {
  if (length <= 0) {
    return 0;
  }
  return ((index % length) + length) % length;
}

function clampIndex(index, length) {
  if (length <= 0) {
    return 0;
  }
  if (index < 0) {
    return 0;
  }
  if (index >= length) {
    return length - 1;
  }
  return index;
}

if (form) {
form.addEventListener("submit", async (event) => {
  event.preventDefault();

  if (fixedURLMode) {
    copyFixedURL();
    return;
  }

  const url = sourceInput.value.trim();
  if (!url) {
    setStatus("Unsupported Instagram URL", "error");
    return;
  }

  submitButton.disabled = true;
  submitButton.setAttribute("aria-busy", "true");
  setStatus("Creating fixed URL...");

  try {
    const response = await fetch("/api/convert", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ url }),
    });
    const payload = await response.json();

    if (!response.ok || !payload.ok) {
      setStatus(payload.error || "Unsupported Instagram URL", "error");
      return;
    }

    sourceInput.value = payload.url;
    sourceInput.readOnly = true;
    sourceInput.focus();
    sourceInput.select();
    fixedURLMode = true;
    submitButton.textContent = "Copy URL";
    if (fixAnotherButton) {
      fixAnotherButton.hidden = false;
    }
    setStatus("Fixed URL created.", "success");
  } catch {
    setStatus("Could not create a fixed URL right now.", "error");
  } finally {
    submitButton.disabled = false;
    submitButton.removeAttribute("aria-busy");
  }
});
}

if (fixAnotherButton) {
  fixAnotherButton.addEventListener("click", () => {
    fixedURLMode = false;
    sourceInput.readOnly = false;
    sourceInput.value = "";
    submitButton.textContent = "Create fixed URL";
    fixAnotherButton.hidden = true;
    setStatus("");
    sourceInput.focus();
  });
}

async function copyFixedURL() {
  const value = sourceInput && sourceInput.value;
  if (!value) {
    return;
  }

  try {
    await navigator.clipboard.writeText(value);
    setStatus("Copied URL.", "success");
  } catch {
    if (sourceInput) {
      sourceInput.focus();
      sourceInput.select();
    }
    setStatus("Select the fixed URL to copy it.", "error");
  }
}

if (galleryRefresh) {
  galleryRefresh.addEventListener("click", loadGallery);
}
if (viewerClose) {
  viewerClose.addEventListener("click", closeViewer);
}
if (viewerPrev) {
  viewerPrev.addEventListener("click", () => showRelativePost(-1));
}
if (viewerNext) {
  viewerNext.addEventListener("click", () => showRelativePost(1));
}
if (viewerMediaPrev) {
  viewerMediaPrev.addEventListener("click", () => showRelativeMedia(-1));
}
if (viewerMediaNext) {
  viewerMediaNext.addEventListener("click", () => showRelativeMedia(1));
}
if (viewerMedia) {
  viewerMedia.addEventListener("wheel", handleViewerWheel, { passive: false });
}

if (viewer) {
  viewer.addEventListener("click", (event) => {
    if (event.target === viewer) {
      closeViewer();
    }
  });

  viewer.addEventListener("close", () => {
    document.body.classList.remove("viewer-open");
  });
}

document.addEventListener("keydown", (event) => {
  if (!viewer || !viewer.open) {
    return;
  }
  if (event.key === "Escape") {
    closeViewer();
  } else if (event.key === "ArrowLeft") {
    showRelativePost(-1);
  } else if (event.key === "ArrowRight") {
    showRelativePost(1);
  } else if (event.key === "ArrowUp") {
    showRelativeMedia(-1);
  } else if (event.key === "ArrowDown") {
    showRelativeMedia(1);
  }
});

if (adminForm) {
adminForm.addEventListener("submit", (event) => {
  event.preventDefault();
  loadAutomation();
});
}

if (adminDialog && typeof adminDialog.showModal === "function") {
  if (!savedAdminToken) {
    adminDialog.showModal();
  }
} else if (adminDialog) {
  adminDialog.setAttribute("open", "");
}

if (instagramForm) {
instagramForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  instagramSave.disabled = true;
  setAutomationStatus("Saving Instagram tracking...");
  try {
    const response = await fetch("/api/automation/config", {
      method: "POST",
      headers: adminHeaders(),
      body: JSON.stringify({
        instagramUsername: instagramInput.value.trim(),
        enabled: automationEnabled.checked,
      }),
    });
    const payload = await readJSON(response);
    if (!response.ok || !payload.ok) {
      setAutomationStatus(payload.error || "Could not save Instagram tracking.", "error");
      return;
    }
    updateAutomationUI(payload);
    setAutomationStatus("Instagram tracking saved.", "success");
    if (galleryGrid) {
      loadGallery();
    }
  } catch {
    setAutomationStatus("Could not save Instagram tracking.", "error");
  } finally {
    instagramSave.disabled = false;
  }
});
}

if (discordForm) {
discordForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const webhookUrl = discordWebhook.value.trim();
  if (!webhookUrl) {
    setAutomationStatus("Discord webhook URL is required.", "error");
    return;
  }

  discordSave.disabled = true;
  setAutomationStatus("Saving Discord webhook...");
  try {
    const response = await fetch("/api/automation/discord/webhook", {
      method: "POST",
      headers: adminHeaders(),
      body: JSON.stringify({ webhookUrl }),
    });
    const payload = await readJSON(response);
    if (!response.ok || !payload.ok) {
      setAutomationStatus(payload.error || "Could not save Discord webhook.", "error");
      return;
    }
    discordWebhook.value = "";
    updateAutomationUI(payload);
    setAutomationStatus("Discord webhook saved.", "success");
  } catch {
    setAutomationStatus("Could not save Discord webhook.", "error");
  } finally {
    discordSave.disabled = false;
  }
});
}

if (discordTest) {
discordTest.addEventListener("click", async () => {
  discordTest.disabled = true;
  setAutomationStatus("Sending Discord test...");
  try {
    const response = await fetch("/api/automation/test", {
      method: "POST",
      headers: adminHeaders(),
      body: "{}",
    });
    const payload = await readJSON(response);
    if (!response.ok || !payload.ok) {
      setAutomationStatus(payload.error || "Discord test failed.", "error");
      return;
    }
    setAutomationStatus("Discord test sent.", "success");
  } catch {
    setAutomationStatus("Discord test failed.", "error");
  } finally {
    discordTest.disabled = false;
  }
});
}

if (discordDisconnect) {
discordDisconnect.addEventListener("click", async () => {
  discordDisconnect.disabled = true;
  setAutomationStatus("Disconnecting Discord...");
  try {
    const response = await fetch("/api/automation/discord/disconnect", {
      method: "POST",
      headers: adminHeaders(),
      body: "{}",
    });
    const payload = await readJSON(response);
    if (!response.ok || !payload.ok) {
      setAutomationStatus(payload.error || "Could not disconnect Discord.", "error");
      return;
    }
    updateAutomationUI(payload);
    setAutomationStatus("Discord disconnected.", "success");
  } catch {
    setAutomationStatus("Could not disconnect Discord.", "error");
  } finally {
    discordDisconnect.disabled = false;
  }
});
}

if (discordOAuth) {
discordOAuth.addEventListener("click", (event) => {
  event.preventDefault();
  if (!adminToken()) {
    setAutomationStatus("Enter admin password.", "error");
    return;
  }
  window.location.href = `/oauth/discord/start?admin_token=${encodeURIComponent(adminToken())}`;
});
}

if (savedAdminToken && adminInput) {
  adminInput.value = savedAdminToken;
  loadAutomation();
}

const pageParams = new URLSearchParams(window.location.search);
if (pageParams.get("discord") === "connected") {
  setAutomationStatus("Discord connected.", "success");
  window.history.replaceState({}, "", window.location.pathname);
} else if (pageParams.get("discord") === "error") {
  setAutomationStatus("Discord connection failed.", "error");
  window.history.replaceState({}, "", window.location.pathname);
}

if (galleryGrid) {
  loadGallery();
}

const form = document.querySelector("#convert-form");
const sourceInput = document.querySelector("#instagram-url");
const submitButton = document.querySelector("#submit-button");
const statusText = document.querySelector("#status");
const resultBlock = document.querySelector("#result-block");
const fixedInput = document.querySelector("#fixed-url");
const copyButton = document.querySelector("#copy-button");

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
const adminInput = document.querySelector("#admin-token");
const adminButton = document.querySelector("#admin-button");
const automationPanel = document.querySelector("#automation-panel");
const automationStatusText = document.querySelector("#automation-status");
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

const galleryState = {
  items: [],
  postIndex: 0,
  mediaIndex: 0,
};

function setStatus(message, kind = "") {
  statusText.textContent = message;
  if (kind) {
    statusText.dataset.kind = kind;
  } else {
    delete statusText.dataset.kind;
  }
}

function setAutomationStatus(message, kind = "") {
  automationStatusText.textContent = message;
  if (kind) {
    automationStatusText.dataset.kind = kind;
  } else {
    delete automationStatusText.dataset.kind;
  }
}

function setGalleryStatus(message, kind = "") {
  galleryStatus.textContent = message;
  if (kind) {
    galleryStatus.dataset.kind = kind;
  } else {
    delete galleryStatus.dataset.kind;
  }
}

function adminToken() {
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
  automationPanel.hidden = false;
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
      return;
    }
    localStorage.setItem(adminStorageKey, adminToken());
    updateAutomationUI(payload);
    setAutomationStatus("Settings unlocked.", "success");
    loadGallery();
  } catch {
    setAutomationStatus("Could not load automation settings.", "error");
  } finally {
    adminButton.disabled = false;
  }
}

async function loadGallery() {
  galleryRefresh.disabled = true;
  setGalleryStatus("Loading gallery...");
  try {
    const response = await fetch("/api/gallery", {
      headers: {
        "Accept": "application/json",
      },
    });
    const payload = await readJSON(response);
    if (!response.ok || !payload.ok) {
      setGalleryStatus(payload.error || "Could not load gallery.", "error");
      galleryGrid.replaceChildren();
      return;
    }
    galleryProfile.textContent = payload.profile || "loonletwow";
    galleryState.items = Array.isArray(payload.items) ? payload.items : [];
    renderGallery();
    if (galleryState.items.length === 0) {
      setGalleryStatus(payload.empty || "No gallery posts yet.");
    } else {
      setGalleryStatus(`${galleryState.items.length} cached posts`);
    }
  } catch {
    galleryGrid.replaceChildren();
    setGalleryStatus("Could not load gallery.", "error");
  } finally {
    galleryRefresh.disabled = false;
  }
}

function renderGallery() {
  const cards = galleryState.items.map((item, index) => {
    const firstMedia = item.media && item.media[0];
    const imageURL = firstMedia && firstMedia.imageUrl;
    const button = document.createElement("button");
    button.className = "gallery-card";
    button.type = "button";
    button.setAttribute("aria-label", `Open post ${item.shortcode}`);
    button.addEventListener("click", () => openViewer(index, 0));

    if (imageURL) {
      const image = document.createElement("img");
      image.src = imageURL;
      image.alt = item.caption ? `@${item.username}: ${item.caption}` : `@${item.username} Instagram post`;
      image.loading = "lazy";
      button.append(image);
    }

    if (item.media && item.media.length > 1) {
      const badge = document.createElement("span");
      badge.className = "gallery-badge";
      badge.textContent = item.media.length;
      button.append(badge);
    }
    return button;
  });
  galleryGrid.replaceChildren(...cards);
}

function openViewer(postIndex, mediaIndex) {
  if (!galleryState.items.length) {
    return;
  }
  galleryState.postIndex = clampIndex(postIndex, galleryState.items.length);
  const post = galleryState.items[galleryState.postIndex];
  galleryState.mediaIndex = clampIndex(mediaIndex, post.media.length);
  renderViewer();
  document.body.classList.add("viewer-open");
  if (typeof viewer.showModal === "function" && !viewer.open) {
    viewer.showModal();
  } else {
    viewer.setAttribute("open", "");
  }
}

function closeViewer() {
  if (viewer.open && typeof viewer.close === "function") {
    viewer.close();
  } else {
    viewer.removeAttribute("open");
  }
  document.body.classList.remove("viewer-open");
}

function renderViewer() {
  const post = galleryState.items[galleryState.postIndex];
  if (!post) {
    return;
  }
  const media = post.media[galleryState.mediaIndex] || post.media[0];
  viewerTitle.textContent = post.username ? `@${post.username}` : "@loonletwow";
  viewerCaption.textContent = post.caption || "No caption cached.";
  viewerFixed.href = post.canonicalUrl || "#";
  viewerOriginal.href = post.originalUrl || "#";

  viewerMedia.replaceChildren(renderViewerMedia(media, post));
  renderViewerDots(post);

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
    video.src = media.videoUrl;
    video.poster = media.imageUrl || "";
    video.controls = true;
    video.playsInline = true;
    video.preload = "metadata";
    video.setAttribute("aria-label", `Video from ${post.shortcode}`);
    return video;
  }

  const image = document.createElement("img");
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
  renderViewer();
}

function showRelativeMedia(offset) {
  const post = galleryState.items[galleryState.postIndex];
  if (!post || post.media.length <= 1) {
    return;
  }
  galleryState.mediaIndex = wrapIndex(galleryState.mediaIndex + offset, post.media.length);
  renderViewer();
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

form.addEventListener("submit", async (event) => {
  event.preventDefault();

  const url = sourceInput.value.trim();
  if (!url) {
    setStatus("Unsupported Instagram URL", "error");
    resultBlock.hidden = true;
    return;
  }

  submitButton.disabled = true;
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
      resultBlock.hidden = true;
      return;
    }

    fixedInput.value = payload.url;
    resultBlock.hidden = false;
    fixedInput.focus();
    fixedInput.select();
    setStatus("Fixed URL created.", "success");
  } catch {
    setStatus("Could not create a fixed URL right now.", "error");
    resultBlock.hidden = true;
  } finally {
    submitButton.disabled = false;
  }
});

copyButton.addEventListener("click", async () => {
  if (!fixedInput.value) {
    return;
  }

  try {
    await navigator.clipboard.writeText(fixedInput.value);
    setStatus("Copied URL.", "success");
  } catch {
    fixedInput.focus();
    fixedInput.select();
    setStatus("Select the fixed URL to copy it.", "error");
  }
});

galleryRefresh.addEventListener("click", loadGallery);
viewerClose.addEventListener("click", closeViewer);
viewerPrev.addEventListener("click", () => showRelativePost(-1));
viewerNext.addEventListener("click", () => showRelativePost(1));
viewerMediaPrev.addEventListener("click", () => showRelativeMedia(-1));
viewerMediaNext.addEventListener("click", () => showRelativeMedia(1));

viewer.addEventListener("click", (event) => {
  if (event.target === viewer) {
    closeViewer();
  }
});

viewer.addEventListener("close", () => {
  document.body.classList.remove("viewer-open");
});

document.addEventListener("keydown", (event) => {
  if (!viewer.open) {
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

adminForm.addEventListener("submit", (event) => {
  event.preventDefault();
  loadAutomation();
});

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
    loadGallery();
  } catch {
    setAutomationStatus("Could not save Instagram tracking.", "error");
  } finally {
    instagramSave.disabled = false;
  }
});

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

discordOAuth.addEventListener("click", (event) => {
  event.preventDefault();
  if (!adminToken()) {
    setAutomationStatus("Enter admin password.", "error");
    return;
  }
  window.location.href = `/oauth/discord/start?admin_token=${encodeURIComponent(adminToken())}`;
});

const savedAdminToken = localStorage.getItem(adminStorageKey);
if (savedAdminToken) {
  adminInput.value = savedAdminToken;
  loadAutomation();
}

const pageParams = new URLSearchParams(window.location.search);
if (pageParams.get("discord") === "connected") {
  setAutomationStatus("Discord connected.", "success");
  window.history.replaceState({}, "", "/");
} else if (pageParams.get("discord") === "error") {
  setAutomationStatus("Discord connection failed.", "error");
  window.history.replaceState({}, "", "/");
}

loadGallery();

const form = document.querySelector("#convert-form");
const sourceInput = document.querySelector("#instagram-url");
const submitButton = document.querySelector("#submit-button");
const statusText = document.querySelector("#status");
const resultBlock = document.querySelector("#result-block");
const fixedInput = document.querySelector("#fixed-url");
const copyButton = document.querySelector("#copy-button");
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
    setAutomationStatus("Enter admin token.", "error");
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
  } catch {
    setAutomationStatus("Could not load automation settings.", "error");
  } finally {
    adminButton.disabled = false;
  }
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

adminForm.addEventListener("submit", (event) => {
  event.preventDefault();
  loadAutomation();
});

instagramForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  instagramSave.disabled = true;
  setAutomationStatus("Saving Instagram source...");
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
      setAutomationStatus(payload.error || "Could not save Instagram source.", "error");
      return;
    }
    updateAutomationUI(payload);
    setAutomationStatus("Instagram source saved.", "success");
  } catch {
    setAutomationStatus("Could not save Instagram source.", "error");
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
    setAutomationStatus("Enter admin token.", "error");
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

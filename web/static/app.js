const form = document.querySelector("#convert-form");
const sourceInput = document.querySelector("#instagram-url");
const submitButton = document.querySelector("#submit-button");
const statusText = document.querySelector("#status");
const resultBlock = document.querySelector("#result-block");
const fixedInput = document.querySelector("#fixed-url");
const copyButton = document.querySelector("#copy-button");

function setStatus(message, kind = "") {
  statusText.textContent = message;
  if (kind) {
    statusText.dataset.kind = kind;
  } else {
    delete statusText.dataset.kind;
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

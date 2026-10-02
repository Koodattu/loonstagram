const LOONDOKU_EMOTES = [
  { name: "Deadge", src: "/static/loondoku/deadge.png" },
  { name: "Gladge", src: "/static/loondoku/gladge.png" },
  { name: "iAsk", src: "/static/loondoku/i-ask.png" },
  { name: "owoBed", src: "/static/loondoku/owo-bed.png" },
  { name: "Pausers", src: "/static/loondoku/pausers.png" },
  { name: "Smadge", src: "/static/loondoku/smadge.png" },
  { name: "Stare", src: "/static/loondoku/stare.png" },
  { name: "sitt", src: "/static/loondoku/sitt.png" },
  { name: "nise", src: "/static/loondoku/nise.png" },
];

const LOONDOKU_SOLUTION = [
  5, 3, 4, 6, 7, 8, 9, 1, 2,
  6, 7, 2, 1, 9, 5, 3, 4, 8,
  1, 9, 8, 3, 4, 2, 5, 6, 7,
  8, 5, 9, 7, 6, 1, 4, 2, 3,
  4, 2, 6, 8, 5, 3, 7, 9, 1,
  7, 1, 3, 9, 2, 4, 8, 5, 6,
  9, 6, 1, 5, 3, 7, 2, 8, 4,
  2, 8, 7, 4, 1, 9, 6, 3, 5,
  3, 4, 5, 2, 8, 6, 1, 7, 9,
];

const LOONDOKU_PUZZLE = [
  5, 3, 0, 0, 7, 0, 0, 0, 0,
  6, 0, 0, 1, 9, 5, 0, 0, 0,
  0, 9, 8, 0, 0, 0, 0, 6, 0,
  8, 0, 0, 0, 6, 0, 0, 0, 3,
  4, 0, 0, 8, 0, 3, 0, 0, 1,
  7, 0, 0, 0, 2, 0, 0, 0, 6,
  0, 6, 0, 0, 0, 0, 2, 8, 0,
  0, 0, 0, 4, 1, 9, 0, 0, 5,
  0, 0, 0, 0, 8, 0, 0, 7, 9,
];

function createLoondokuPuzzle(random = Math.random) {
  function shuffle(items) {
    const result = items.slice();
    for (let index = result.length - 1; index > 0; index -= 1) {
      const other = Math.floor(random() * (index + 1));
      [result[index], result[other]] = [result[other], result[index]];
    }
    return result;
  }

  // These symmetries preserve the seed puzzle's difficulty and unique solution.
  const order = () => shuffle([0, 1, 2]).flatMap((group) =>
    shuffle([0, 1, 2]).map((offset) => group * 3 + offset));
  const rows = order();
  const columns = order();
  const symbols = [0, ...shuffle([1, 2, 3, 4, 5, 6, 7, 8, 9])];
  const transpose = random() < 0.5;
  const transform = (grid) => rows.flatMap((row) => columns.map((column) =>
    symbols[grid[transpose ? column * 9 + row : row * 9 + column]]));

  return { puzzle: transform(LOONDOKU_PUZZLE), solution: transform(LOONDOKU_SOLUTION) };
}

const board = document.querySelector("#loondoku-board");
const palette = document.querySelector("#loondoku-palette");
const status = document.querySelector("#loondoku-status");
const checkButton = document.querySelector("#loondoku-check");
const emptyButton = document.querySelector("#loondoku-empty");
const resetButton = document.querySelector("#loondoku-reset");
const resetDialog = document.querySelector("#loondoku-reset-dialog");
const newButton = document.querySelector("#loondoku-new");
const soundButton = document.querySelector("#loondoku-sound");
const completion = document.querySelector("#loondoku-completion");
const playAgainButton = document.querySelector("#loondoku-play-again");
const fireworks = document.querySelector("#loondoku-fireworks");
const dialogTitle = document.querySelector("#loondoku-reset-title");
const dialogCopy = document.querySelector("#loondoku-reset-copy");
const dialogConfirm = document.querySelector("#loondoku-reset-confirm");

if (board && palette && status && checkButton && emptyButton && resetButton && resetDialog
  && newButton && soundButton && completion && playAgainButton && fireworks
  && dialogTitle && dialogCopy && dialogConfirm) {
  const cells = [];
  const choices = [];
  let game = createLoondokuPuzzle();
  let values = game.puzzle.slice();
  let selectedIndex = values.findIndex((value) => value === 0);
  let showMistakes = false;
  let isComplete = false;
  let pendingAction = "reset";
  let dialogTrigger = resetButton;
  let fireworksTimeout;
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
  const soundStorageKey = "loondoku-sound";
  let soundEnabled = true;
  let audioContext;
  let soundGeneration = 0;
  const activeSounds = new Set();

  try {
    soundEnabled = window.localStorage.getItem(soundStorageKey) !== "off";
  } catch {
    // Sound still works for this visit when browser storage is unavailable.
  }

  function renderSoundButton() {
    soundButton.textContent = soundEnabled ? "Sound on" : "Sound off";
    soundButton.setAttribute("aria-pressed", String(soundEnabled));
  }

  function stopSounds() {
    soundGeneration += 1;
    activeSounds.forEach((oscillator) => oscillator.stop());
    activeSounds.clear();
  }

  async function playSound(kind) {
    if (!soundEnabled) return;
    const generation = soundGeneration;
    try {
      const AudioContext = window.AudioContext || window.webkitAudioContext;
      if (!AudioContext) return;
      if (!audioContext) audioContext = new AudioContext();
      if (audioContext.state === "suspended") await audioContext.resume();
      if (!soundEnabled || generation !== soundGeneration || audioContext.state !== "running") return;

      const notes = {
        place: [660],
        clear: [330],
        error: [180, 140],
        check: [523.25, 659.25],
        complete: [523.25, 659.25, 783.99, 1046.5, 1318.5],
      }[kind];
      const duration = kind === "complete" ? 0.4 : 0.12;
      notes.forEach((frequency, index) => {
        const oscillator = audioContext.createOscillator();
        const gain = audioContext.createGain();
        const start = audioContext.currentTime + 0.01 + index * 0.12;
        oscillator.type = "sine";
        oscillator.frequency.value = frequency;
        gain.gain.setValueAtTime(0, start);
        gain.gain.linearRampToValueAtTime(0.06, start + 0.01);
        gain.gain.exponentialRampToValueAtTime(0.001, start + duration);
        oscillator.connect(gain);
        gain.connect(audioContext.destination);
        oscillator.onended = () => {
          activeSounds.delete(oscillator);
          oscillator.disconnect();
          gain.disconnect();
        };
        activeSounds.add(oscillator);
        oscillator.start(start);
        oscillator.stop(start + duration);
      });
    } catch {
      // Audio is optional; a blocked or unavailable device must not interrupt play.
    }
  }

  function clearFireworks() {
    window.clearTimeout(fireworksTimeout);
    fireworks.replaceChildren();
  }

  function celebrate() {
    playSound("complete");
    if (reducedMotion.matches) return;
    const colors = ["#ff8fc8", "#ffe08a", "#a6f0c3", "#c6b4ff"];
    const bursts = [[25, 35], [75, 30], [50, 60]];
    bursts.forEach(([left, top], burst) => {
      for (let index = 0; index < 24; index += 1) {
        const spark = document.createElement("span");
        const angle = (index / 24) * Math.PI * 2;
        const distance = 65 + Math.random() * 80;
        spark.className = "loondoku-spark";
        spark.style.left = `${left}%`;
        spark.style.top = `${top}%`;
        spark.style.backgroundColor = colors[index % colors.length];
        spark.style.setProperty("--spark-x", `${Math.cos(angle) * distance}px`);
        spark.style.setProperty("--spark-y", `${Math.sin(angle) * distance}px`);
        spark.style.animationDelay = `${burst * 0.3}s`;
        fireworks.append(spark);
      }
    });
    fireworksTimeout = window.setTimeout(clearFireworks, 2200);
  }

  reducedMotion.addEventListener("change", () => {
    if (reducedMotion.matches) clearFireworks();
  });

  function emoteImage(value) {
    const emote = LOONDOKU_EMOTES[value - 1];
    const image = document.createElement("img");
    image.src = emote.src;
    image.alt = "";
    image.draggable = false;
    return image;
  }

  function setStatus(message, kind = "") {
    status.textContent = message;
    status.dataset.kind = kind;
  }

  function cellLabel(index) {
    const row = Math.floor(index / 9) + 1;
    const column = (index % 9) + 1;
    const value = values[index];
    if (!value) {
      return `Row ${row}, column ${column}, empty`;
    }
    const fixed = game.puzzle[index] !== 0 ? ", fixed" : "";
    return `Row ${row}, column ${column}, ${LOONDOKU_EMOTES[value - 1].name}${fixed}`;
  }

  function conflictIndexes() {
    const conflicts = new Set();
    const groups = [];

    for (let row = 0; row < 9; row += 1) {
      groups.push(Array.from({ length: 9 }, (_, column) => row * 9 + column));
    }
    for (let column = 0; column < 9; column += 1) {
      groups.push(Array.from({ length: 9 }, (_, row) => row * 9 + column));
    }
    for (let boxRow = 0; boxRow < 3; boxRow += 1) {
      for (let boxColumn = 0; boxColumn < 3; boxColumn += 1) {
        groups.push(Array.from({ length: 9 }, (_, offset) => {
          const row = boxRow * 3 + Math.floor(offset / 3);
          const column = boxColumn * 3 + (offset % 3);
          return row * 9 + column;
        }));
      }
    }

    groups.forEach((group) => {
      const positions = new Map();
      group.forEach((index) => {
        const value = values[index];
        if (!value) return;
        const matches = positions.get(value) || [];
        matches.push(index);
        positions.set(value, matches);
      });
      positions.forEach((indexes) => {
        if (indexes.length > 1) {
          indexes.forEach((index) => conflicts.add(index));
        }
      });
    });

    return conflicts;
  }

  function render() {
    const conflicts = conflictIndexes();
    const selectedValue = values[selectedIndex];

    cells.forEach((cell, index) => {
      const value = values[index];
      cell.replaceChildren();
      if (value) cell.append(emoteImage(value));
      cell.classList.toggle("is-given", game.puzzle[index] !== 0);
      cell.classList.toggle("is-selected", !isComplete && index === selectedIndex);
      cell.classList.toggle("is-matching", !isComplete && Boolean(selectedValue) && value === selectedValue && index !== selectedIndex);
      cell.classList.toggle("is-conflict", conflicts.has(index));
      cell.classList.toggle("is-wrong", showMistakes && value !== 0 && value !== game.solution[index]);
      cell.tabIndex = index === selectedIndex ? 0 : -1;
      cell.setAttribute("aria-label", cellLabel(index));
      cell.setAttribute("aria-readonly", isComplete || game.puzzle[index] !== 0 ? "true" : "false");
    });

    choices.forEach((choice, index) => {
      choice.setAttribute("aria-pressed", selectedValue === index + 1 ? "true" : "false");
      choice.disabled = isComplete;
    });
    emptyButton.disabled = isComplete || game.puzzle[selectedIndex] !== 0 || selectedValue === 0;
    checkButton.disabled = isComplete;
  }

  function updateProgress() {
    const remaining = values.filter((value) => value === 0).length;
    const conflicts = conflictIndexes();
    const solved = values.every((value, index) => value === game.solution[index]);

    board.classList.toggle("is-complete", solved);
    if (solved) {
      setStatus("Solved! Every Pepe found its place.", "success");
      if (!isComplete) {
        isComplete = true;
        completion.hidden = false;
        render();
        completion.focus({ preventScroll: true });
        celebrate();
      }
    } else if (conflicts.size > 0) {
      setStatus("That Pepe is repeated in a row, column, or box.", "error");
    } else if (remaining === 1) {
      setStatus("1 square left.");
    } else {
      setStatus(`${remaining} squares left.`);
    }
  }

  function selectCell(index, focus = false) {
    selectedIndex = index;
    render();
    if (focus) cells[index].focus();
  }

  function placeValue(value) {
    if (isComplete) return;
    if (game.puzzle[selectedIndex] !== 0) {
      setStatus("Choose an empty square first.");
      return;
    }
    if (values[selectedIndex] === value) return;
    values[selectedIndex] = value;
    showMistakes = false;
    render();
    const changedCell = cells[selectedIndex];
    const feedbackClass = value === 0 ? "is-cleared" : "is-placed";
    changedCell.classList.add(feedbackClass);
    window.setTimeout(() => changedCell.classList.remove(feedbackClass), 220);
    updateProgress();
    if (!isComplete) {
      playSound(value === 0 ? "clear" : conflictIndexes().has(selectedIndex) ? "error" : "place");
    }
  }

  function moveSelection(rowOffset, columnOffset) {
    const row = Math.floor(selectedIndex / 9);
    const column = selectedIndex % 9;
    const nextRow = Math.min(8, Math.max(0, row + rowOffset));
    const nextColumn = Math.min(8, Math.max(0, column + columnOffset));
    selectCell(nextRow * 9 + nextColumn, true);
  }

  game.puzzle.forEach((_, index) => {
    const cell = document.createElement("button");
    cell.type = "button";
    cell.className = "loondoku-cell";
    cell.setAttribute("role", "gridcell");
    cell.addEventListener("click", () => selectCell(index));
    cell.addEventListener("keydown", (event) => {
      if (event.key === "ArrowUp") moveSelection(-1, 0);
      else if (event.key === "ArrowDown") moveSelection(1, 0);
      else if (event.key === "ArrowLeft") moveSelection(0, -1);
      else if (event.key === "ArrowRight") moveSelection(0, 1);
      else return;
      event.preventDefault();
    });
    board.append(cell);
    cells.push(cell);
  });

  LOONDOKU_EMOTES.forEach((emote, index) => {
    const choice = document.createElement("button");
    choice.type = "button";
    choice.className = "loondoku-choice";
    choice.title = emote.name;
    choice.setAttribute("aria-label", `Place ${emote.name}, key ${index + 1}`);
    choice.setAttribute("aria-pressed", "false");
    choice.append(emoteImage(index + 1));
    const key = document.createElement("span");
    key.className = "loondoku-key";
    key.textContent = String(index + 1);
    key.setAttribute("aria-hidden", "true");
    choice.append(key);
    choice.addEventListener("click", () => placeValue(index + 1));
    palette.append(choice);
    choices.push(choice);
  });

  checkButton.addEventListener("click", () => {
    if (isComplete) return;
    showMistakes = true;
    render();
    const remaining = values.filter((value) => value === 0).length;
    const wrong = values.filter((value, index) => value !== 0 && value !== game.solution[index]).length;
    if (wrong > 0) {
      setStatus(`${wrong} ${wrong === 1 ? "Pepe is" : "Pepes are"} out of place.`, "error");
      playSound("error");
    } else if (remaining > 0) {
      setStatus(`Looking good. ${remaining} ${remaining === 1 ? "square" : "squares"} left.`);
      playSound("check");
    } else {
      updateProgress();
    }
  });

  function resetGame(newPuzzle = false) {
    stopSounds();
    clearFireworks();
    if (newPuzzle) game = createLoondokuPuzzle();
    values = game.puzzle.slice();
    selectedIndex = values.findIndex((value) => value === 0);
    showMistakes = false;
    isComplete = false;
    completion.hidden = true;
    board.classList.remove("is-complete");
    render();
    updateProgress();
    cells[selectedIndex].focus();
  }

  emptyButton.addEventListener("click", () => placeValue(0));

  function requestRestart(action, trigger) {
    const hasEntries = values.some((value, index) => value !== game.puzzle[index]);
    if (isComplete || !hasEntries) {
      resetGame(action === "new");
      return;
    }
    pendingAction = action;
    dialogTrigger = trigger;
    dialogTitle.textContent = action === "new" ? "Start a new puzzle?" : "Reset this puzzle?";
    dialogCopy.textContent = action === "new"
      ? "Your progress will be replaced with a fresh puzzle."
      : "Your entries will be cleared. You'll keep the same puzzle.";
    dialogConfirm.textContent = action === "new" ? "New puzzle" : "Reset puzzle";
    resetDialog.returnValue = "";
    resetDialog.showModal();
  }

  resetButton.addEventListener("click", () => requestRestart("reset", resetButton));
  newButton.addEventListener("click", () => requestRestart("new", newButton));
  playAgainButton.addEventListener("click", () => resetGame(true));

  resetDialog.addEventListener("close", () => {
    if (resetDialog.returnValue === "reset") {
      resetGame(pendingAction === "new");
    } else {
      dialogTrigger.focus();
    }
  });

  document.addEventListener("keydown", (event) => {
    if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey || resetDialog.open) return;
    if (/^[1-9]$/.test(event.key)) {
      placeValue(Number(event.key));
    } else if (event.key === "Backspace" || event.key === "Delete" || event.key === "0") {
      placeValue(0);
    } else {
      return;
    }
    event.preventDefault();
  });

  soundButton.addEventListener("click", () => {
    soundEnabled = !soundEnabled;
    stopSounds();
    renderSoundButton();
    try {
      window.localStorage.setItem(soundStorageKey, soundEnabled ? "on" : "off");
    } catch {
      // Keep the control usable even when preferences cannot be saved.
    }
    if (soundEnabled) playSound("check");
  });

  renderSoundButton();
  render();
  updateProgress();
}

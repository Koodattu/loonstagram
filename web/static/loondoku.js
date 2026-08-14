const LOONDOKU_EMOTES = [
  { name: "peepoBlushPhone", src: "/static/loondoku/peepo-blush-phone.png" },
  { name: "peepoCute", src: "/static/loondoku/peepo-cute.png" },
  { name: "peepoFlower", src: "/static/loondoku/peepo-flower.png" },
  { name: "peepoNotes", src: "/static/loondoku/peepo-notes.png" },
  { name: "peepoShrug", src: "/static/loondoku/peepo-shrug.png" },
  { name: "peepoSip", src: "/static/loondoku/peepo-sip.png" },
  { name: "Comfi", src: "/static/loondoku/comfi.png" },
  { name: "cowoffee", src: "/static/loondoku/cowoffee.png" },
  { name: "cuteSitFriendship", src: "/static/loondoku/cute-sit-friendship.png" },
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

const board = document.querySelector("#loondoku-board");
const palette = document.querySelector("#loondoku-palette");
const status = document.querySelector("#loondoku-status");
const checkButton = document.querySelector("#loondoku-check");
const resetButton = document.querySelector("#loondoku-reset");

if (board && palette && status && checkButton && resetButton) {
  const cells = [];
  const choices = [];
  let values = LOONDOKU_PUZZLE.slice();
  let selectedIndex = values.findIndex((value) => value === 0);
  let showMistakes = false;

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
    const fixed = LOONDOKU_PUZZLE[index] !== 0 ? ", fixed" : "";
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
      cell.classList.toggle("is-given", LOONDOKU_PUZZLE[index] !== 0);
      cell.classList.toggle("is-selected", index === selectedIndex);
      cell.classList.toggle("is-matching", Boolean(selectedValue) && value === selectedValue && index !== selectedIndex);
      cell.classList.toggle("is-conflict", conflicts.has(index));
      cell.classList.toggle("is-wrong", showMistakes && value !== 0 && value !== LOONDOKU_SOLUTION[index]);
      cell.tabIndex = index === selectedIndex ? 0 : -1;
      cell.setAttribute("aria-label", cellLabel(index));
      cell.setAttribute("aria-readonly", LOONDOKU_PUZZLE[index] !== 0 ? "true" : "false");
    });

    choices.forEach((choice, index) => {
      choice.setAttribute("aria-pressed", selectedValue === index + 1 ? "true" : "false");
    });
  }

  function updateProgress() {
    const remaining = values.filter((value) => value === 0).length;
    const conflicts = conflictIndexes();
    const solved = values.every((value, index) => value === LOONDOKU_SOLUTION[index]);

    board.classList.toggle("is-complete", solved);
    if (solved) {
      setStatus("Solved! Every Pepe found its place.", "success");
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
    if (LOONDOKU_PUZZLE[selectedIndex] !== 0) {
      setStatus("Choose an empty square first.");
      return;
    }
    values[selectedIndex] = value;
    showMistakes = false;
    render();
    updateProgress();
  }

  function moveSelection(rowOffset, columnOffset) {
    const row = Math.floor(selectedIndex / 9);
    const column = selectedIndex % 9;
    const nextRow = Math.min(8, Math.max(0, row + rowOffset));
    const nextColumn = Math.min(8, Math.max(0, column + columnOffset));
    selectCell(nextRow * 9 + nextColumn, true);
  }

  LOONDOKU_PUZZLE.forEach((_, index) => {
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
      else if (/^[1-9]$/.test(event.key)) placeValue(Number(event.key));
      else if (event.key === "Backspace" || event.key === "Delete" || event.key === "0") placeValue(0);
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
    choice.setAttribute("aria-label", `Place ${emote.name}`);
    choice.setAttribute("aria-pressed", "false");
    choice.append(emoteImage(index + 1));
    choice.addEventListener("click", () => placeValue(index + 1));
    palette.append(choice);
    choices.push(choice);
  });

  checkButton.addEventListener("click", () => {
    showMistakes = true;
    render();
    const remaining = values.filter((value) => value === 0).length;
    const wrong = values.filter((value, index) => value !== 0 && value !== LOONDOKU_SOLUTION[index]).length;
    if (wrong > 0) {
      setStatus(`${wrong} ${wrong === 1 ? "Pepe is" : "Pepes are"} out of place.`, "error");
    } else if (remaining > 0) {
      setStatus(`Looking good. ${remaining} ${remaining === 1 ? "square" : "squares"} left.`);
    } else {
      board.classList.add("is-complete");
      setStatus("Solved! Every Pepe found its place.", "success");
    }
  });

  resetButton.addEventListener("click", () => {
    values = LOONDOKU_PUZZLE.slice();
    selectedIndex = values.findIndex((value) => value === 0);
    showMistakes = false;
    board.classList.remove("is-complete");
    render();
    updateProgress();
    cells[selectedIndex].focus();
  });

  render();
  updateProgress();
}

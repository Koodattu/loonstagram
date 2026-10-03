const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const source = readFileSync(path.join(__dirname, "static/loondoku.js"), "utf8");
const template = readFileSync(path.join(__dirname, "templates/loondoku.html"), "utf8");

function randomGenerator(seed) {
  return () => {
    seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
    return seed / 2 ** 32;
  };
}

// An independent solver checks uniqueness as well as the generated answer.
function solutionsFor(puzzle) {
  const values = Array.from(puzzle);
  const solutions = [];
  function visit() {
    let bestIndex = -1;
    let bestOptions = [];
    for (let index = 0; index < 81; index += 1) {
      if (values[index]) continue;
      const row = Math.floor(index / 9);
      const column = index % 9;
      const used = new Set();
      for (let offset = 0; offset < 9; offset += 1) {
        used.add(values[row * 9 + offset]);
        used.add(values[offset * 9 + column]);
        used.add(values[(Math.floor(row / 3) * 3 + Math.floor(offset / 3)) * 9
          + Math.floor(column / 3) * 3 + offset % 3]);
      }
      const options = [1, 2, 3, 4, 5, 6, 7, 8, 9].filter((value) => !used.has(value));
      if (!options.length) return;
      if (bestIndex < 0 || options.length < bestOptions.length) {
        bestIndex = index;
        bestOptions = options;
      }
    }
    if (bestIndex < 0) {
      solutions.push(values.slice());
      return;
    }
    for (const value of bestOptions) {
      values[bestIndex] = value;
      visit();
      if (solutions.length > 1) break;
    }
    values[bestIndex] = 0;
  }
  visit();
  return solutions;
}

// Small DOM/audio doubles keep these behavior tests dependency-free.
class Element {
  constructor() {
    this.children = [];
    this.attributes = new Map();
    this.listeners = new Map();
    this.dataset = {};
    this.style = { setProperty() {} };
    this.hidden = false;
    this.disabled = false;
    this.open = false;
    const classes = new Set();
    this.classList = {
      add: (...names) => names.forEach((name) => classes.add(name)),
      remove: (...names) => names.forEach((name) => classes.delete(name)),
      contains: (name) => classes.has(name),
      toggle: (name, on) => on ? classes.add(name) : classes.delete(name),
    };
  }
  append(child) { this.children.push(child); }
  replaceChildren() { this.children = []; }
  setAttribute(name, value) { this.attributes.set(name, value); }
  getAttribute(name) { return this.attributes.get(name); }
  addEventListener(type, listener) {
    if (!this.listeners.has(type)) this.listeners.set(type, []);
    this.listeners.get(type).push(listener);
  }
  dispatch(type, event = {}) {
    for (const listener of this.listeners.get(type) || []) listener(event);
  }
  click() { if (!this.disabled) this.dispatch("click"); }
  focus() { this.focused = true; }
  showModal() { this.open = true; }
  close(value = "") {
    this.open = false;
    this.returnValue = value;
    this.dispatch("close");
  }
}

function startGame({ storage = new Map(), reducedMotion = false, audio = true, audioState = "running" } = {}) {
  const elements = new Map(Array.from(template.matchAll(/id="([^"]+)"/g), (match) => [match[1], new Element()]));
  const get = (name) => elements.get(`loondoku-${name}`);
  get("completion").hidden = true;
  const document = new Element();
  document.querySelector = (selector) => elements.get(selector.slice(1));
  document.createElement = () => new Element();
  const motion = new Element();
  motion.matches = reducedMotion;
  const timers = new Map();
  let timerID = 0;
  const notes = [];
  const contexts = [];
  class AudioContext {
    constructor() {
      this.state = audioState;
      this.currentTime = 0;
      contexts.push(this);
    }
    async resume() { this.state = "running"; }
    createOscillator() {
      const note = {
        frequency: {}, connect() {}, disconnect() {}, start() {},
        stop(at) { this.stoppedAt = at; },
      };
      notes.push(note);
      return note;
    }
    createGain() {
      return {
        gain: { setValueAtTime() {}, linearRampToValueAtTime() {}, exponentialRampToValueAtTime() {} },
        connect() {}, disconnect() {},
      };
    }
  }
  const window = {
    matchMedia: () => motion,
    localStorage: { getItem: (key) => storage.get(key), setItem: (key, value) => storage.set(key, value) },
    setTimeout(callback, delay) { timers.set(++timerID, { callback, delay }); return timerID; },
    clearTimeout(id) { timers.delete(id); },
    AudioContext: audio ? AudioContext : undefined,
  };
  const context = vm.createContext({ document, window, Math: Object.assign(Object.create(Math), { random: randomGenerator(42) }) });
  vm.runInContext(source, context);
  const emotes = vm.runInContext("LOONDOKU_EMOTES.map((emote) => emote.src)", context);
  const cells = get("board").children;
  const values = () => cells.map((cell) => {
    const symbol = cell.children[0];
    if (!symbol) return 0;
    return symbol.src ? emotes.indexOf(symbol.src) + 1 : Number(symbol.textContent);
  });
  const initial = values();
  const [solution] = solutionsFor(initial);
  function key(key) {
    document.dispatch("keydown", { key, preventDefault() {} });
  }
  function place(index, value) {
    cells[index].click();
    key(String(value));
  }
  function solve() {
    initial.forEach((value, index) => { if (!value) place(index, solution[index]); });
  }
  return { get, cells, values, initial, solution, key, place, solve, notes, contexts, timers, motion };
}

test("randomized puzzles have distinct layouts and exactly one correct solution", () => {
  const context = vm.createContext({ document: { querySelector: () => null } });
  vm.runInContext(source, context);
  const original = vm.runInContext("LOONDOKU_PUZZLE.join(',') + '/' + LOONDOKU_SOLUTION.join(',')", context);
  const layouts = new Set();
  const cluePatterns = new Set();
  for (let seed = 1; seed <= 100; seed += 1) {
    const { puzzle, solution } = context.createLoondokuPuzzle(randomGenerator(seed));
    assert.equal(puzzle.length, 81);
    assert.equal(solution.length, 81);
    assert.equal(puzzle.filter(Boolean).length, 30);
    assert.ok(puzzle.every((value, index) => value === 0 || value === solution[index]));
    const solved = solutionsFor(puzzle);
    assert.equal(solved.length, 1);
    assert.deepEqual(solved[0], Array.from(solution));
    for (let index = 0; index < 9; index += 1) {
      assert.equal(new Set(solution.slice(index * 9, index * 9 + 9)).size, 9);
      assert.equal(new Set(Array.from({ length: 9 }, (_, row) => solution[row * 9 + index])).size, 9);
      const box = Array.from({ length: 9 }, (_, offset) => solution[
        (Math.floor(index / 3) * 3 + Math.floor(offset / 3)) * 9 + index % 3 * 3 + offset % 3]);
      assert.equal(new Set(box).size, 9);
    }
    layouts.add(puzzle.join(","));
    cluePatterns.add(puzzle.map(Boolean).join(","));
  }
  assert.equal(layouts.size, 100);
  assert.ok(cluePatterns.size > 90);
  assert.equal(vm.runInContext("LOONDOKU_PUZZLE.join(',') + '/' + LOONDOKU_SOLUTION.join(',')", context), original);
});

test("only a correct finish celebrates, once, and the solved board stays complete", () => {
  const game = startGame();
  assert.equal(game.contexts.length, 0, "no audio autoplay on page load");
  const empty = game.initial.map((value, index) => value ? -1 : index).filter((index) => index >= 0);
  for (const index of empty.slice(0, -1)) game.place(index, game.solution[index]);
  assert.equal(game.get("completion").hidden, true);
  assert.equal(game.get("fireworks").children.length, 0);

  const last = empty.at(-1);
  game.place(last, game.solution[last] % 9 + 1);
  game.get("check").click();
  assert.equal(game.get("status").dataset.kind, "error");
  assert.equal(game.get("completion").hidden, true);
  assert.equal(game.get("fireworks").children.length, 0);

  const before = game.notes.length;
  game.place(last, game.solution[last]);
  assert.equal(game.get("completion").hidden, false);
  assert.equal(game.get("status").dataset.kind, "success");
  assert.equal(game.get("board").classList.contains("is-complete"), true);
  assert.equal(game.get("fireworks").children.length, 72);
  assert.equal(game.notes.length - before, 5, "one victory chime without a placement tone");
  assert.equal(game.get("check").disabled, true);
  assert.equal(game.get("empty").disabled, true);
  assert.ok(game.get("palette").children.every((choice) => choice.disabled));
  assert.ok(game.cells.every((cell) => cell.getAttribute("aria-readonly") === "true"));

  game.key("Backspace");
  game.place(last, 1);
  game.get("check").click();
  assert.deepEqual(game.values(), game.solution);
  assert.equal(game.notes.length - before, 5);
  const cleanup = Array.from(game.timers.values()).find((timer) => timer.delay === 2200);
  cleanup.callback();
  assert.equal(game.get("fireworks").children.length, 0);
  assert.equal(game.get("completion").hidden, false);
});

test("reset preserves the puzzle; new puzzle confirms before discarding progress", () => {
  const game = startGame();
  const index = game.initial.indexOf(0);
  game.place(index, game.solution[index]);
  game.get("new").click();
  assert.equal(game.get("reset-dialog").open, true);
  assert.equal(game.get("reset-title").textContent, "Start a new puzzle?");
  game.key("2");
  assert.equal(game.values()[index], game.solution[index], "keyboard entry is ignored in the dialog");
  game.get("reset-dialog").close();
  assert.equal(game.values()[index], game.solution[index]);
  assert.equal(game.get("new").focused, true);

  game.get("reset").click();
  assert.equal(game.get("reset-title").textContent, "Reset this puzzle?");
  game.get("reset-dialog").close("reset");
  assert.deepEqual(game.values(), game.initial);

  game.place(index, game.solution[index]);
  game.get("new").click();
  game.get("reset-dialog").close("reset");
  assert.notDeepEqual(game.values(), game.initial);
  assert.equal(game.values().filter(Boolean).length, 30);
  assert.equal(solutionsFor(game.values()).length, 1);
});

test("play again clears completion and effects and re-enables play on a fresh puzzle", () => {
  const game = startGame();
  game.solve();
  game.get("play-again").click();
  assert.equal(game.get("completion").hidden, true);
  assert.equal(game.get("board").classList.contains("is-complete"), false);
  assert.equal(game.get("fireworks").children.length, 0);
  assert.equal(game.get("check").disabled, false);
  assert.ok(game.get("palette").children.every((choice) => !choice.disabled));
  assert.notDeepEqual(game.values(), game.initial);
  assert.equal(game.get("status").textContent, "51 squares left.");
  assert.ok(game.notes.every((note) => note.stoppedAt === undefined), "scheduled sound is stopped");
  const index = game.values().indexOf(0);
  game.place(index, 1);
  assert.equal(game.values()[index], 1);
});

test("sound can be muted immediately and the preference survives a new visit", () => {
  const storage = new Map();
  const game = startGame({ storage });
  game.place(game.initial.indexOf(0), 1);
  assert.ok(game.notes.length > 0);
  game.get("sound").click();
  assert.ok(game.notes.every((note) => note.stoppedAt === undefined));
  assert.equal(game.get("sound").getAttribute("aria-pressed"), "false");
  const before = game.notes.length;
  game.solve();
  assert.equal(game.notes.length, before);
  assert.equal(game.get("completion").hidden, false);

  const next = startGame({ storage });
  assert.equal(next.get("sound").textContent, "Sound off");
  next.solve();
  assert.equal(next.contexts.length, 0);
  next.get("sound").click();
  assert.equal(storage.get("loondoku-sound"), "on");
  assert.equal(next.notes.length, 2);
});

test("muting while audio resumes cancels the pending sound", async () => {
  const game = startGame({ audioState: "suspended" });
  game.place(game.initial.indexOf(0), 1);
  game.get("sound").click();
  await Promise.resolve();
  assert.equal(game.notes.length, 0);
});

test("switching symbols preserves entries, selection, conflicts, and checked mistakes", () => {
  const game = startGame();
  assert.equal(game.get("emotes").getAttribute("aria-pressed"), "true");
  const blanks = game.initial.map((value, index) => value ? -1 : index).filter((index) => index >= 0);
  game.place(blanks[0], game.solution[blanks[0]]);
  game.place(blanks[1], game.solution[blanks[1]] % 9 + 1);
  game.get("check").click();
  const snapshot = () => ({
    values: game.values(),
    cells: game.cells.map((cell) => ({
      classes: ["is-given", "is-selected", "is-matching", "is-conflict", "is-wrong"]
        .map((name) => cell.classList.contains(name)),
      tabIndex: cell.tabIndex,
      readonly: cell.getAttribute("aria-readonly"),
    })),
    message: game.get("status").textContent,
    kind: game.get("status").dataset.kind,
    sounds: game.notes.length,
    timers: game.timers.size,
  });
  const before = snapshot();

  game.get("numbers").click();
  assert.deepEqual(snapshot(), before);
  assert.equal(game.get("numbers").getAttribute("aria-pressed"), "true");
  assert.equal(game.get("emotes").getAttribute("aria-pressed"), "false");
  assert.equal(game.get("palette-title").textContent, "Pick a number");
  assert.equal(game.get("palette").getAttribute("aria-label"), "Number choices");
  assert.ok(game.cells.every((cell) => !cell.children.length || cell.children[0].className === "loondoku-number"));
  game.get("palette").children.forEach((choice, index) => {
    assert.equal(choice.children.length, 1);
    assert.equal(choice.children[0].textContent, String(index + 1));
    assert.equal(choice.getAttribute("aria-label"), `Place ${index + 1}, key ${index + 1}`);
  });
  assert.ok(game.cells[blanks[0]].getAttribute("aria-label").endsWith(`, ${game.solution[blanks[0]]}`));

  game.get("emotes").click();
  assert.deepEqual(snapshot(), before);
  assert.equal(game.get("palette-title").textContent, "Pick a Pepe");
  assert.ok(game.cells.every((cell) => !cell.children.length || cell.children[0].src));
  assert.ok(game.get("palette").children.every((choice) => choice.children.length === 2));

  game.get("numbers").click();
  game.key("0");
  assert.equal(game.values()[blanks[1]], 0, "keyboard clearing still uses the selected square");
  game.get("palette").children[game.solution[blanks[1]] - 1].click();
  assert.equal(game.values()[blanks[1]], game.solution[blanks[1]], "number choices still place values");
  game.get("emotes").click();
  assert.equal(game.values()[blanks[1]], game.solution[blanks[1]]);
});

test("numbers mode can finish a puzzle and changing symbols preserves completion", () => {
  const game = startGame();
  const first = game.initial.indexOf(0);
  game.place(first, game.solution[first]);
  game.get("numbers").click();
  game.solve();
  const notes = game.notes.length;
  const fireworks = game.get("fireworks").children.slice();
  for (const mode of ["emotes", "numbers"]) {
    game.get(mode).click();
    assert.deepEqual(game.values(), game.solution);
    assert.equal(game.get("completion").hidden, false);
    assert.equal(game.get("status").dataset.kind, "success");
    assert.equal(game.get("board").classList.contains("is-complete"), true);
    assert.ok(game.get("palette").children.every((choice) => choice.disabled));
    assert.equal(game.notes.length, notes, "changing symbols does not replay the victory sound");
    assert.deepEqual(game.get("fireworks").children, fireworks);
  }
  game.get("reset").click();
  assert.deepEqual(game.values(), game.initial);
  assert.equal(game.get("completion").hidden, true);
  assert.equal(game.get("numbers").getAttribute("aria-pressed"), "true");
});

test("the display preference survives new puzzles and visits", () => {
  const storage = new Map();
  const game = startGame({ storage });
  game.get("numbers").click();
  game.get("new").click();
  assert.equal(game.get("numbers").getAttribute("aria-pressed"), "true");
  assert.equal(storage.get("loondoku-display"), "numbers");
  const next = startGame({ storage });
  assert.equal(next.get("numbers").getAttribute("aria-pressed"), "true");
  assert.ok(next.cells.every((cell) => !cell.children.length || cell.children[0].className === "loondoku-number"));
  next.get("emotes").click();
  assert.equal(startGame({ storage }).get("emotes").getAttribute("aria-pressed"), "true");
});

test("reduced motion skips fireworks but still confirms success", () => {
  const game = startGame({ reducedMotion: true });
  game.solve();
  assert.equal(game.get("completion").hidden, false);
  assert.equal(game.get("status").dataset.kind, "success");
  assert.equal(game.get("fireworks").children.length, 0);
  assert.ok(game.notes.length > 0);
});

test("switching to reduced motion removes active fireworks", () => {
  const game = startGame();
  game.solve();
  game.motion.matches = true;
  game.motion.dispatch("change");
  assert.equal(game.get("fireworks").children.length, 0);
  assert.equal(game.get("completion").hidden, false);
});

test("blocked storage and missing audio support do not prevent play or completion", () => {
  const storage = { get() { throw new Error("blocked"); }, set() { throw new Error("blocked"); } };
  const game = startGame({ storage, audio: false });
  game.get("sound").click();
  game.get("sound").click();
  game.get("numbers").click();
  assert.equal(game.get("numbers").getAttribute("aria-pressed"), "true");
  game.solve();
  assert.equal(game.get("completion").hidden, false);
  assert.equal(game.get("status").dataset.kind, "success");
});

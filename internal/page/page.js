"use strict";

(function () {
  const SVG_NAMESPACE = "http://www.w3.org/2000/svg";
  const SIZE = { width: 176, height: 72, stubHeight: 40, gapX: 40, gapY: 72, margin: 32, frame: 24 };
  const ARROW_OFFSET = 4;
  const TOP = "";

  const model = JSON.parse(document.getElementById("model").textContent);
  const elements = {};
  for (const id of ["crumbs", "map", "canvas", "arrows", "lines", "nodes", "card", "breaks"]) {
    elements[id] = document.getElementById(id);
  }

  // A key is also the URL fragment of its node. The prefix keeps a box and a class apart, because
  // a one-word module path can give them the same id.
  const boxKey = (id) => "box=" + id;
  const classKey = (id) => "class=" + id;

  const { nodes, children } = buildTree();
  const neighbours = buildNeighbours();
  const chains = new Map();
  const levels = new Map();
  let view = { level: null, card: null };
  let hovered = null;
  let selected = null;
  let drawnBreaks = {};

  function buildTree() {
    const nodes = new Map();
    for (const box of model.packages) {
      nodes.set(boxKey(box.id), {
        key: boxKey(box.id),
        kind: "box",
        name: box.name,
        label: box.id.replace(/^:/, ""),
        parent: box.parent ? boxKey(box.parent) : TOP,
        count: box.packageCount,
        duplicates: box.duplicateCount,
        grade: box.grade,
      });
    }
    for (const item of model.classes) {
      nodes.set(classKey(item.id), {
        key: classKey(item.id),
        kind: item.foreign ? "foreign" : "class",
        name: item.name,
        label: item.foreign ? item.id : item.path === "." ? item.name : item.path,
        parent: item.package ? boxKey(item.package) : TOP,
        duplicates: item.duplicateCount,
        grade: item.grade,
        item,
      });
    }
    const children = new Map([[TOP, []]]);
    for (const node of nodes.values()) {
      if (!children.has(node.parent)) children.set(node.parent, []);
      children.get(node.parent).push(node);
    }
    const placeInOrder = (node) => {
      const place = (model.order ?? []).indexOf(node.name);
      return place < 0 ? Infinity : place;
    };
    for (const [parent, list] of children) {
      list.sort((a, b) => {
        if (parent === TOP && placeInOrder(a) !== placeInOrder(b)) {
          return placeInOrder(a) < placeInOrder(b) ? -1 : 1;
        }
        return compareText(a.name, b.name) || compareText(a.key, b.key);
      });
    }
    const keys = new Map();
    for (const [parent, list] of children) keys.set(parent, list.map((node) => node.key));
    return { nodes, children: keys };
  }

  // Only dependency edges, because a composition edge repeats an import that is already listed.
  function buildNeighbours() {
    const neighbours = new Map();
    const of = (id) => {
      const key = classKey(id);
      if (!neighbours.has(key)) neighbours.set(key, { imports: [], importers: [] });
      return neighbours.get(key);
    };
    for (const edge of model.edges) {
      if (edge.kind !== "dependency") continue;
      of(edge.from).imports.push(edge);
      of(edge.to).importers.push(edge);
    }
    return neighbours;
  }

  // Not localeCompare: two readers of one shared link must see the same order.
  function compareText(a, b) {
    return a < b ? -1 : a > b ? 1 : 0;
  }

  function childrenOf(key) {
    return children.get(key) ?? [];
  }

  function isBox(key) {
    return key === TOP || nodes.get(key).kind === "box";
  }

  // A box with one child has nothing to choose from, so no level ever stops on it.
  function opensThrough(key) {
    return key !== TOP && isBox(key) && childrenOf(key).length === 1;
  }

  function openTarget(key) {
    while (opensThrough(key)) key = childrenOf(key)[0];
    return key;
  }

  function nearestLevel(key) {
    while (opensThrough(key)) key = nodes.get(key).parent;
    return key;
  }

  function viewOf(key) {
    if (key !== TOP && !nodes.has(key)) return { level: TOP, card: null };
    const target = openTarget(key);
    if (isBox(target)) return { level: target, card: null };
    return { level: nearestLevel(nodes.get(target).parent), card: target };
  }

  function chainOf(key) {
    if (key === TOP) return [TOP];
    let chain = chains.get(key);
    if (!chain) {
      chain = [...chainOf(nodes.get(key).parent), key];
      chains.set(key, chain);
    }
    return chain;
  }

  // visibleEnd finds what stands for a class at a level: a child of the level, or else the box
  // outside the level that is the nearest to it.
  function visibleEnd(classId, levelChain) {
    const chain = chainOf(classKey(classId));
    let depth = 0;
    while (depth < levelChain.length && chain[depth] === levelChain[depth]) depth++;
    return { key: chain[depth], outside: depth < levelChain.length };
  }

  // levelOf is the only code that walks the class edges. It runs once per level, because a
  // monorepo has too many edges to walk on each draw.
  function levelOf(level) {
    if (levels.has(level)) return levels.get(level);
    const levelChain = chainOf(level);
    const arrows = new Map();
    const outside = new Set();
    for (const edge of model.edges) {
      const from = visibleEnd(edge.from, levelChain);
      const to = visibleEnd(edge.to, levelChain);
      if ((from.outside && to.outside) || from.key === to.key) continue;
      if (from.outside) outside.add(from.key);
      if (to.outside) outside.add(to.key);
      const id = from.key + "\0" + to.key;
      if (!arrows.has(id)) {
        arrows.set(id, { from: from.key, to: to.key, inside: !from.outside && !to.outside, edges: [], breaks: [], loop: false });
      }
      arrows.get(id).edges.push(edge);
      if (edge.violations) arrows.get(id).breaks.push(edge);
    }
    const list = [...arrows.values()];
    const importing = new Set(list.map((arrow) => arrow.from));
    const byLabel = (a, b) => compareText(nodes.get(a).label, nodes.get(b).label);
    const stubs = [...outside].sort(byLabel);
    const rows = rowsOf(childrenOf(level), list.filter((arrow) => arrow.inside));
    // A second walk, so that an edge box changes neither the rows nor which inside arrow is marked.
    // The edge boxes go first, so that a loop through one is marked on the arrow back to it.
    const everyKey = [...stubs, ...childrenOf(level)];
    markLoops(everyKey, outgoingOf(everyKey, list.filter((arrow) => !arrow.loop)));
    const result = {
      rows,
      importers: stubs.filter((key) => importing.has(key)),
      imported: stubs.filter((key) => !importing.has(key)),
      arrows: list,
    };
    levels.set(level, result);
    return result;
  }

  function outgoingOf(keys, arrows) {
    const outgoing = new Map(keys.map((key) => [key, []]));
    for (const arrow of arrows) outgoing.get(arrow.from).push(arrow);
    return outgoing;
  }

  // An arrow that reaches back to a key on the current walk closes a loop, so it is marked. The
  // result lists each key after the keys it imports.
  function markLoops(keys, outgoing) {
    const walking = new Set();
    const finished = new Set();
    const order = [];
    const walk = (key) => {
      walking.add(key);
      for (const arrow of outgoing.get(key)) {
        if (walking.has(arrow.to)) arrow.loop = true;
        else if (!finished.has(arrow.to)) walk(arrow.to);
      }
      walking.delete(key);
      finished.add(key);
      order.push(key);
    };
    for (const key of keys) if (!finished.has(key)) walk(key);
    return order;
  }

  // rowsOf puts each key one row below the deepest key that imports it. A marked arrow is left out
  // of the row count, because a loop has no deepest key.
  function rowsOf(keys, arrows) {
    const outgoing = outgoingOf(keys, arrows);
    const order = markLoops(keys, outgoing);

    const depth = new Map(keys.map((key) => [key, 0]));
    for (const key of order.reverse()) {
      for (const arrow of outgoing.get(key)) {
        if (!arrow.loop) depth.set(arrow.to, Math.max(depth.get(arrow.to), depth.get(key) + 1));
      }
    }
    const rows = [];
    for (const key of keys) (rows[depth.get(key)] ??= []).push(key);
    return rows;
  }

  function arrange(level, content) {
    const free = elements.map.clientWidth - 2 * (SIZE.margin + SIZE.frame) + SIZE.gapX;
    const perLine = Math.max(3, Math.floor(free / (SIZE.width + SIZE.gapX)));
    const lines = [];
    const places = new Map();
    let y = SIZE.margin;

    const placeBand = (keys, height) => {
      for (let start = 0; start < keys.length; start += perLine) {
        const line = keys.slice(start, start + perLine).map((key, index) => {
          const place = { x: index * (SIZE.width + SIZE.gapX), y, width: SIZE.width, height };
          place.oval = nodes.get(key).kind === "foreign";
          places.set(key, place);
          return place;
        });
        lines.push(line);
        y += height + SIZE.gapY;
      }
    };

    const inset = level === TOP ? 0 : SIZE.frame;
    placeBand(content.importers, SIZE.stubHeight);
    const frameTop = y;
    y += inset;
    for (const row of content.rows) placeBand(row, SIZE.height);
    const frameBottom = y - SIZE.gapY + inset;
    y = frameBottom + SIZE.gapY;
    placeBand(content.imported, SIZE.stubHeight);

    const lineWidth = (line) => line.length * (SIZE.width + SIZE.gapX) - SIZE.gapX;
    const widest = Math.max(0, ...lines.map(lineWidth));
    for (const line of lines) {
      const shift = SIZE.margin + SIZE.frame + (widest - lineWidth(line)) / 2;
      for (const place of line) place.x += shift;
    }
    return {
      places,
      width: widest + 2 * (SIZE.margin + SIZE.frame),
      height: y - SIZE.gapY + SIZE.margin,
      frame: inset && { x: SIZE.margin, y: frameTop, width: widest + 2 * SIZE.frame, height: frameBottom - frameTop },
    };
  }

  function borderPoint(place, shift, dx, dy) {
    const [halfWidth, halfHeight] = [place.width / 2, place.height / 2];
    const scale = place.oval
      ? 1 / Math.hypot(dx / halfWidth, dy / halfHeight)
      : Math.min(
          (halfWidth - Math.sign(dx) * shift.x) / Math.abs(dx),
          (halfHeight - Math.sign(dy) * shift.y) / Math.abs(dy),
        );
    return { x: place.x + halfWidth + shift.x + dx * scale, y: place.y + halfHeight + shift.y + dy * scale };
  }

  function drawLine(start, end, className) {
    const line = document.createElementNS(SVG_NAMESPACE, "line");
    line.setAttribute("x1", start.x);
    line.setAttribute("y1", start.y);
    line.setAttribute("x2", end.x);
    line.setAttribute("y2", end.y);
    line.setAttribute("class", className);
    elements.lines.append(line);
    return line;
  }

  function drawBreakTarget(arrow, start, end) {
    const target = drawLine(start, end, "arrow-hit");
    target.setAttribute("tabindex", "0");
    target.setAttribute("role", "button");
    const show = () => hoverArrow(arrow);
    const hide = () => hoverArrow(null);
    target.addEventListener("mouseenter", show);
    target.addEventListener("mouseleave", hide);
    target.addEventListener("focus", show);
    target.addEventListener("blur", hide);
    target.addEventListener("click", () => selectArrow(arrow));
    target.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") selectArrow(arrow);
    });
    return target;
  }

  function drawArrow(arrow, places) {
    const [from, to] = [places.get(arrow.from), places.get(arrow.to)];
    const dx = to.x + to.width / 2 - (from.x + from.width / 2);
    const dy = to.y + to.height / 2 - (from.y + from.height / 2);
    // Each arrow moves to its own right, so that two arrows between one pair of boxes don't overlap.
    const length = Math.hypot(dx, dy);
    const shift = { x: (-dy / length) * ARROW_OFFSET, y: (dx / length) * ARROW_OFFSET };
    const start = borderPoint(from, shift, dx, dy);
    const end = borderPoint(to, shift, -dx, -dy);

    const isBreak = arrow.breaks.length > 0;
    const line = drawLine(start, end, "arrow" + (arrow.loop ? " loop" : "") + (isBreak ? " break" : ""));
    line.setAttribute("marker-end", isBreak ? "url(#head-break)" : arrow.loop ? "url(#head-loop)" : "url(#head)");
    arrow.line = line;
    const title = document.createElementNS(SVG_NAMESPACE, "title");
    title.textContent = describeArrow(arrow);
    (isBreak ? drawBreakTarget(arrow, start, end) : line).append(title);

    if (arrow.loop) {
      const label = document.createElementNS(SVG_NAMESPACE, "text");
      label.setAttribute("x", (start.x + end.x) / 2);
      label.setAttribute("y", (start.y + end.y) / 2);
      label.setAttribute("class", "loop-label");
      label.textContent = "loop";
      elements.lines.append(label);
    }
  }

  function describeArrow(arrow) {
    const imports = arrow.edges.filter((edge) => edge.kind === "dependency").length;
    const text = `${nodes.get(arrow.from).label} imports ${nodes.get(arrow.to).label}`;
    const count = `${imports} ${imports === 1 ? "import" : "imports"}`;
    const breaks = arrow.breaks.length;
    const parts = [`${text} (${count}).`];
    if (arrow.loop) parts.push("This arrow closes a loop.");
    if (breaks > 0) parts.push(`${breaks} ${breaks === 1 ? "import breaks" : "imports break"} an arch-go rule.`);
    return parts.join(" ");
  }

  function sizeText(node) {
    if (node.kind === "box") return `${node.count} ${node.count === 1 ? "package" : "packages"}`;
    return node.kind === "foreign" ? "outside module" : "package";
  }

  // A count of 0 still prints: it tells a clean box from a run with no dup report.
  function duplicatesText(node) {
    if (node.duplicates === undefined) return "";
    return `${node.duplicates} duplicated ${node.duplicates === 1 ? "function" : "functions"}`;
  }

  function drawNode(node, place, isStub) {
    const button = document.createElement("button");
    button.type = "button";
    button.dataset.key = node.key;
    button.className = "node";
    if (node.kind === "foreign") button.classList.add("foreign");
    if (isStub) button.classList.add("stub");
    if (node.grade) button.classList.add("grade-" + node.grade);
    button.style.left = place.x + "px";
    button.style.top = place.y + "px";
    button.style.width = place.width + "px";
    button.style.height = place.height + "px";
    const duplicates = duplicatesText(node);
    button.title = `${node.label}: ${sizeText(node)}` + (node.grade ? `, ${node.grade} risk` : "") + (duplicates ? `, ${duplicates}` : "");

    const name = document.createElement("span");
    name.className = "name";
    name.textContent = isStub ? node.label : node.name;
    button.append(name);
    if (!isStub) {
      const size = document.createElement("span");
      size.className = "size";
      size.textContent = sizeText(node);
      button.append(size);
      if (duplicates) {
        const count = document.createElement("span");
        count.className = "size";
        count.textContent = duplicates;
        button.append(count);
      }
    }
    if (node.grade === "red") {
      const cue = document.createElement("span");
      cue.className = "cue";
      cue.textContent = "!";
      button.append(cue);
    }
    elements.nodes.append(button);
  }

  function drawLevel(level) {
    const content = levelOf(level);
    const layout = arrange(level, content);
    elements.canvas.style.width = layout.width + "px";
    elements.canvas.style.height = layout.height + "px";
    elements.arrows.setAttribute("width", layout.width);
    elements.arrows.setAttribute("height", layout.height);
    elements.lines.replaceChildren();
    elements.nodes.replaceChildren();
    hovered = null;
    selected = null;
    drawBreaks();
    elements.map.scrollTo(0, 0);

    if (model.classes.length === 0) {
      const empty = document.createElement("p");
      empty.id = "empty";
      empty.textContent = "godeps found no packages.";
      elements.nodes.append(empty);
      return;
    }
    if (layout.frame) {
      const frame = document.createElement("div");
      frame.className = "frame";
      frame.textContent = nodes.get(level).label;
      frame.style.left = layout.frame.x + "px";
      frame.style.top = layout.frame.y + "px";
      frame.style.width = layout.frame.width + "px";
      frame.style.height = layout.frame.height + "px";
      elements.nodes.append(frame);
    }
    for (const arrow of content.arrows) drawArrow(arrow, layout.places);
    const stubs = new Set([...content.importers, ...content.imported]);
    for (const [key, place] of layout.places) drawNode(nodes.get(key), place, stubs.has(key));
  }

  function crumbsOf(current) {
    const crumbs = [{ key: TOP, label: "top" }];
    let names = [];
    for (const key of chainOf(current.level).slice(1)) {
      names.push(nodes.get(key).name);
      if (!opensThrough(key)) {
        crumbs.push({ key, label: names.join("/") });
        names = [];
      }
    }
    if (current.card) crumbs.push({ key: current.card, label: nodes.get(current.card).name });
    return crumbs;
  }

  function fragmentOf(key) {
    return "#" + encodeURI(key);
  }

  function drawCrumbs() {
    const crumbs = crumbsOf(view);
    const parts = [];
    crumbs.forEach((crumb, index) => {
      if (index > 0) {
        const separator = document.createElement("span");
        separator.className = "separator";
        separator.textContent = "/";
        parts.push(separator);
      }
      const isLast = index === crumbs.length - 1;
      const part = document.createElement(isLast ? "span" : "a");
      part.textContent = crumb.label;
      if (isLast) part.setAttribute("aria-current", "page");
      else part.href = fragmentOf(crumb.key);
      parts.push(part);
    });
    elements.crumbs.replaceChildren(...parts);
  }

  function element(tag, className, ...content) {
    const made = document.createElement(tag);
    if (className) made.className = className;
    made.append(...content.filter(Boolean));
    return made;
  }

  function editorLink(item, text) {
    const link = element("a", "", text);
    link.href = item.url;
    return link;
  }

  // Neovim has no URL handler, so a terminal user copies this text instead of following the link.
  function whereText(item) {
    return element("code", "where", `${item.file}:${item.line}`);
  }

  function cardLink(key, text) {
    const link = element("a", "", text);
    link.href = fragmentOf(key);
    return link;
  }

  function section(title, entries) {
    if (entries.length === 0) return null;
    return element("section", "", element("h3", "", `${title} (${entries.length})`), element("ul", "entries", ...entries));
  }

  function typeEntry(type) {
    const members = [
      ...(type.fields ?? []).map((field) => `${field.name} ${field.type}`),
      ...(type.methods ?? []).map((method) => method.name + (method.signature ?? "")),
    ];
    return element(
      "li",
      "type",
      editorLink(type, type.name),
      type.kind === "other" ? " " : ` ${type.kind} `,
      whereText(type),
      members.length > 0 && element("ul", "members", ...members.map((text) => element("li", "", element("code", "", text)))),
    );
  }

  // An op with a grade and no numbers is missing from the report. An op with neither is ignored, an
  // assembly stub, or a run with no report, so there is no risk to state.
  function scoreText(op) {
    if (op.crap !== undefined) {
      const coverage = (op.coverage * 100).toFixed(1);
      return `CRAP ${op.crap.toFixed(1)}, complexity ${op.complexity}, coverage ${coverage}%`;
    }
    if (op.ignored) return "Excused from crap by //goalie:ignore";
    return op.grade ? "No crap score, so this counts as high risk" : "";
  }

  function duplicateEntry(duplicate) {
    return element(
      "li",
      "duplicate",
      "Duplicate of ",
      editorLink(duplicate, duplicate.name),
      " ",
      whereText(duplicate),
      ` score ${duplicate.score.toFixed(2)}, `,
      duplicate.id ? cardLink(classKey(duplicate.id), "open card") : "not on the map",
    );
  }

  function opEntry(op) {
    const score = scoreText(op);
    const duplicates = op.duplicates ?? [];
    return element(
      "li",
      "op",
      editorLink(op, op.name),
      " ",
      whereText(op),
      score && element("div", "", element("span", "score grade-" + op.grade, op.grade === "red" ? "! " + score : score)),
      duplicates.length > 0 && element("ul", "members", ...duplicates.map(duplicateEntry)),
    );
  }

  function neighbourEntry(id, edge) {
    const key = classKey(id);
    const mark = edge.violations && element("span", "break-mark", " ! breaks a rule");
    if (mark) mark.title = edge.violations.map((violation) => violation.rule).join("\n");
    return element("li", "", cardLink(key, nodes.get(key).label), mark);
  }

  function drawCard() {
    elements.card.hidden = !view.card;
    if (!view.card) return;
    const node = nodes.get(view.card);
    const { imports, importers } = neighbours.get(node.key) ?? { imports: [], importers: [] };
    const close = element("button", "", "Close");
    close.type = "button";
    close.addEventListener("click", goUp);
    elements.card.replaceChildren(
      ...[
        element("h2", "", node.name),
        element("p", "", node.label),
        close,
        section("Types", node.item.types.map(typeEntry)),
        section("Functions", node.item.ops.map(opEntry)),
        section("Imports", imports.map((edge) => neighbourEntry(edge.to, edge))),
        section("Imported by", importers.map((edge) => neighbourEntry(edge.from, edge))),
      ].filter(Boolean),
    );
    elements.card.scrollTop = 0;
  }

  function breakEntry(edge, violation) {
    const [from, to] = [classKey(edge.from), classKey(edge.to)];
    return element(
      "li",
      "",
      cardLink(from, nodes.get(from).label),
      " imports ",
      cardLink(to, nodes.get(to).label),
      element("ul", "members", ...violation.details.map((detail) => element("li", "", detail))),
    );
  }

  // One rule can flag many imports inside a folder arrow, so the rule text prints once.
  function ruleSections(arrow) {
    const rules = new Map();
    for (const edge of arrow.breaks) {
      for (const violation of edge.violations) {
        if (!rules.has(violation.rule)) rules.set(violation.rule, []);
        rules.get(violation.rule).push(breakEntry(edge, violation));
      }
    }
    return [...rules].map(([rule, entries]) => element("section", "", element("h3", "", rule), element("ul", "entries", ...entries)));
  }

  function drawBreaks() {
    const arrow = hovered ?? selected;
    elements.breaks.hidden = !arrow;
    if (selected) selected.line.classList.add("selected");
    if (!arrow) return;
    elements.breaks.classList.toggle("selected", arrow === selected);
    // A press on a link blurs the arrow, which lands here. Rebuilding the panel between the press
    // and the release would swallow the click.
    if (drawnBreaks.arrow === arrow && drawnBreaks.selected === (arrow === selected)) return;
    drawnBreaks = { arrow, selected: arrow === selected };
    const heading = element("h2", "", `${nodes.get(arrow.from).label} imports ${nodes.get(arrow.to).label}`);
    const close = element("button", "", "Close");
    close.type = "button";
    close.addEventListener("click", () => selectArrow(null));
    elements.breaks.replaceChildren(heading, ...ruleSections(arrow), ...(arrow === selected ? [close] : []));
  }

  function hoverArrow(arrow) {
    hovered = arrow;
    drawBreaks();
  }

  // A second select of one arrow clears it. hovered clears too, or the panel would stay under the
  // pointer and hide that the select was undone.
  function selectArrow(arrow) {
    if (selected) selected.line.classList.remove("selected");
    selected = arrow === selected ? null : arrow;
    hovered = null;
    drawBreaks();
  }

  function go(key) {
    location.hash = encodeURI(key);
  }

  function goUp() {
    if (view.card) go(view.level);
    else if (view.level !== TOP) go(nearestLevel(nodes.get(view.level).parent));
  }

  function readFragment() {
    try {
      return decodeURIComponent(location.hash.slice(1));
    } catch {
      return TOP;
    }
  }

  function render() {
    const next = viewOf(readFragment());
    const levelChanged = next.level !== view.level;
    view = next;
    if (levelChanged) drawLevel(view.level);
    drawCrumbs();
    drawCard();
  }

  function openFrom(event) {
    const button = event.target.closest(".node");
    if (button) go(openTarget(button.dataset.key));
  }

  elements.nodes.addEventListener("dblclick", openFrom);
  elements.nodes.addEventListener("keydown", (event) => {
    if (event.key === "Enter") openFrom(event);
  });
  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    if (selected) selectArrow(null);
    else goUp();
  });
  window.addEventListener("hashchange", render);
  render();
})();

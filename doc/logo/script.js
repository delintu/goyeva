function createLogo(container = document.body, options = {}) {
  const config = {
    size: options.size || 400,
    backgroundColor: options.backgroundColor || "#000000",
    textColor: options.textColor || "white",
    text: options.text || "yv",
    fontFamily: options.fontFamily || "Cambria",
    className: options.className || "",
    id: options.id || "",
  };

  const backDiv = document.createElement("div");
  backDiv.className = config.className;
  if (config.id) backDiv.id = config.id;

  Object.assign(backDiv.style, {
    backgroundColor: config.backgroundColor,
    width: `${config.size}px`,
    height: `${config.size}px`,
    display: "flex",
    justifyContent: "end",
    alignItems: "end",
  });

  const textDiv = document.createElement("div");
  textDiv.textContent = config.text;

  const innerSize = Math.floor(config.size * 0.8);
  const fontSize = Math.floor(innerSize * 0.8);

  Object.assign(textDiv.style, {
    color: config.textColor,
    width: `${innerSize}px`,
    height: `${innerSize}px`,
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
    fontFamily: config.fontFamily,
    fontSize: `${fontSize}px`,
  });

  backDiv.appendChild(textDiv);

  if (container && container.appendChild) {
    container.appendChild(backDiv);
  }

  return backDiv;
}

document.addEventListener("DOMContentLoaded", function () {
  createLogo(document.body, { text: "YV", backgroundColor: "#1c8ca8" });
});

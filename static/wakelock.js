let wakeLock = null;

const appendLog = (message, isError = false) => {
  const logOutput = document.getElementById("log_output");
  if (logOutput) {
    const code = logOutput.querySelector("code");
    if (code) {
      const logEntry = document.createElement("div");
      const textNode = document.createTextNode(message);
      logEntry.appendChild(textNode);

      if (isError) {
        logEntry.style.color = "orange";
      }

      code.appendChild(logEntry);
    }
  }
};

function log(message) {
  appendLog(message);
}

function error(message) {
  appendLog(message, true);
}

const requestWakeLock = async () => {
  try {
    wakeLock = await navigator.wakeLock.request("screen");
    log("Wake Lock is active");
    return wakeLock;
  } catch (err) {
    error(`Failed to request Wake Lock: ${err.name}, ${err.message}`);
    return null;
  }
};

const handleVisibilityChange = async () => {
  if (document.visibilityState === "visible") {
    await requestWakeLock();
  }
};

const initWakeLock = async () => {
  await requestWakeLock();
  document.addEventListener("visibilitychange", handleVisibilityChange);
};

// Automatically execute when the script is loaded
initWakeLock().then(() => {
  log("Wake Lock initialized");
}).catch((err) => {
  error(`Error initializing Wake Lock: ${err.message}`);
});

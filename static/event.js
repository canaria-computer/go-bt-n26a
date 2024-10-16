const resultDetails = document.getElementById("resultDetails");
resultDetails.innerHTML =
  window.sessionStorage.getItem("resultDetailsBackup") || "";

const currentScanResultOutput = document.getElementById(
  "currentScanResultOutput",
);
const deviceCount = document.getElementById("deviceCount");

const eventSource = new EventSource("/events");
eventSource.onmessage = function (event) {
  const devices = JSON.parse(event.data);
  const [
    eventId,
    resultText,
    count,
  ] = [
      event.lastEventId,
      JSON.stringify(devices, null, 2),
      Object.keys(devices).length,
    ];

  if (currentScanResultOutput) {
    currentScanResultOutput.textContent = resultText;
  }
  if (deviceCount) {
    deviceCount.textContent = count;
  }

  const resultDetailsTemplate = document.getElementById(
    "resultDetailsTemplate",
  );
  const content = resultDetailsTemplate.content;
  content.querySelector("summary").textContent = "#" + eventId;
  content.querySelector("code").textContent = resultText;
  content.querySelector("data-count").textContent = count + "台";
  const clone = document.importNode(content, true);
  resultDetails.prepend(clone);

  window.sessionStorage.setItem("resultDetailsBackup", resultDetails.innerHTML);
};
eventSource.onopen = function () {
  document.getElementById("loading-bar").classList.add("enable");
  const systemStatusBar = document.getElementById("SystemStatus");
  systemStatusBar.classList.add("OK");
  systemStatusBar.classList.remove("ERR");
  document.getElementById("SystemStatusText").textContent = "Normal";
};

eventSource.onerror = function () {
  const systemStatusBar = document.getElementById("SystemStatus");
  systemStatusBar.classList.remove("OK");
  systemStatusBar.classList.add("ERR");
  document.getElementById("SystemStatusText").textContent =
    "An error occurred while attempting to sync.";
  document.getElementById("loading-bar").classList.remove("enable");
};

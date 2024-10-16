const format = new Intl.DateTimeFormat(window.navigator.language, {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
});

const i = setInterval(() => {
  const target = document.querySelector("#time");
  const timeString = format.format(new Date());
  target.textContent = timeString;
}, 850);

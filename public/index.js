let state = {
  currentPhoto: document.querySelector(".images img"),
};

function showModal() {
  let photoSrc = state.currentPhoto.src;
  photoSrc = photoSrc.replaceAll("/preview", "");
  document.querySelector("#full-image").src = photoSrc;
  document.querySelector("#image-viewer").style.display = "block";
  refreshArrows();
}

function hideModal() {
  document.querySelector("#image-viewer").style.display = "none";
}

function refreshArrows() {
  let nextButton = document.querySelector("#image-viewer .next");
  if (state.currentPhoto.nextElementSibling) {
    nextButton.style.display = "block";
  } else {
    nextButton.style.display = "none";
  }

  let prevButton = document.querySelector("#image-viewer .prev");
  if (state.currentPhoto.previousElementSibling) {
    prevButton.style.display = "block";
  } else {
    prevButton.style.display = "none";
  }
}

function changeImage(previous = false) {
  if (previous) {
    if (state.currentPhoto.previousElementSibling) {
      state.currentPhoto = state.currentPhoto.previousElementSibling;
    }
  } else {
    if (state.currentPhoto.nextElementSibling) {
      state.currentPhoto = state.currentPhoto.nextElementSibling;
    }
  }

  showModal();
}

document.querySelectorAll(".images img").forEach((e) => {
  e.addEventListener("click", () => {
    state.currentPhoto = e;
    showModal();
  });
});

document
  .querySelector("#image-viewer .next")
  .addEventListener("click", () => changeImage());

document
  .querySelector("#image-viewer .prev")
  .addEventListener("click", () => changeImage(true));

document.querySelector("#image-viewer .close").addEventListener("click", () => {
  hideModal();
});

document.body.addEventListener("keydown", (event) => {
  const key = event.key;
  switch (key) {
    case "ArrowLeft":
      changeImage(true);
      break;
    case "ArrowRight":
      changeImage();
      break;
    case "Escape":
      hideModal();
      break;
  }
});

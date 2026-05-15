const form = document.querySelector("#signup-form");
const message = document.querySelector("#form-message");
const authForm = document.querySelector("#auth-form");
const authMessage = document.querySelector("#auth-message");
const tabs = document.querySelectorAll(".tab");
const companyField = document.querySelector(".company-field");
const accountState = document.querySelector(".account-state");
const accountName = document.querySelector("#account-name");
const accountEmail = document.querySelector("#account-email");
const logoutButton = document.querySelector("#logout-button");
const newsletterForm = document.querySelector(".newsletter-form");
const newsletterMessage = document.querySelector(".newsletter-message");
let authMode = "register";
let currentUser = null;

const checkoutState = new URLSearchParams(window.location.search).get("checkout");
if (checkoutState === "success") {
  messageForPage("Payment received. We will email you with setup next steps.");
}
if (checkoutState === "cancelled") {
  messageForPage("Checkout cancelled. You can restart whenever you are ready.");
}

function messageForPage(text) {
  const signup = document.querySelector("#signup");
  if (!signup) return;
  signup.scrollIntoView({ block: "center" });
  const p = document.querySelector("#form-message");
  if (p) p.textContent = text;
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  message.textContent = "Creating checkout...";
  const data = Object.fromEntries(new FormData(form).entries());

  try {
    const res = await fetch("/checkout/create", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
    const body = await res.json();
    if (!res.ok) {
      throw new Error(body.message || body.error || "Checkout is unavailable");
    }
    window.location.href = body.url;
  } catch (err) {
    message.textContent = err.message;
  }
});

tabs.forEach((tab) => {
  tab.addEventListener("click", () => {
    authMode = tab.dataset.mode;
    tabs.forEach((item) => item.classList.toggle("active", item === tab));
    companyField.hidden = authMode === "login";
    companyField.querySelector("input").required = authMode === "register";
    authForm.querySelector("button").textContent = authMode === "login" ? "Login" : "Create account";
    authMessage.textContent = "";
  });
});

authForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  authMessage.textContent = authMode === "login" ? "Logging in..." : "Creating account...";
  const data = Object.fromEntries(new FormData(authForm).entries());
  const endpoint = authMode === "login" ? "/api/login" : "/api/register";
  try {
    const res = await fetch(endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
    const body = await res.json();
    if (!res.ok) throw new Error(body.error || "Account request failed");
    setUser(body.user);
    authMessage.textContent = authMode === "login" ? "Logged in." : "Account created.";
    form.email.value = body.user.email;
    form.company.value = body.user.company;
  } catch (err) {
    authMessage.textContent = err.message;
  }
});

logoutButton?.addEventListener("click", async () => {
  await fetch("/api/logout", { method: "POST" });
  setUser(null);
});

newsletterForm?.addEventListener("submit", (event) => {
  event.preventDefault();
  const email = new FormData(newsletterForm).get("email");
  newsletterMessage.textContent = email ? "Thanks. We will send occasional product updates." : "Enter an email to subscribe.";
  newsletterForm.reset();
});

async function loadCurrentUser() {
  try {
    const res = await fetch("/api/me");
    if (!res.ok) return;
    const body = await res.json();
    setUser(body.user);
  } catch {
    setUser(null);
  }
}

function setUser(user) {
  currentUser = user;
  if (accountState && authForm) {
    accountState.hidden = !user;
    authForm.hidden = !!user;
  }
  if (user) {
    accountName.textContent = user.company;
    accountEmail.textContent = user.email;
  }
}

loadCurrentUser();

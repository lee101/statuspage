describe("statuspage.app.nz landing page", function () {
  it("renders the core brand and pricing", function () {
    expect(document.title).toContain("statuspage.app.nz");
    expect(document.querySelector(".brand strong").textContent).toContain("statuspage.app.nz");
    expect(document.body.textContent).toContain("$190/year");
    expect(document.querySelector("#billing-form input[name='plan'][value='annual']").checked).toBeTrue();
    expect(document.querySelector("#billing-form input[name='plan'][value='monthly']")).not.toBeNull();
  });

  it("renders the example status dashboard", function () {
    expect(document.querySelector(".dashboard")).not.toBeNull();
    expect(document.body.textContent).toContain("All systems operational");
    expect(document.querySelectorAll(".monitor-table .row").length).toBeGreaterThan(3);
  });

  it("renders real Applied AI product links", function () {
    var customers = document.querySelector("#customers");
    expect(customers).not.toBeNull();
    expect(customers.textContent).toContain("Applied AI products");
    expect(customers.textContent).toContain("text-generator.io");
    expect(customers.textContent).toContain("netwrck.com");
    expect(customers.textContent).toContain("simplexgen.com");
    expect(customers.textContent).not.toContain("devmate.co.nz");
    expect(customers.textContent).not.toContain("Add your business");
  });

  it("renders login and signup controls", function () {
    expect(document.querySelector("#auth-form")).not.toBeNull();
    expect(document.querySelector("[data-mode='login']")).not.toBeNull();
    expect(document.querySelector("[data-mode='forgot']")).not.toBeNull();
    expect(document.querySelector("#signup-form")).not.toBeNull();
    expect(document.querySelector("#billing-dialog")).not.toBeNull();
  });

  it("switches the account panel to login mode", function () {
    document.querySelector("[data-mode='login']").click();
    expect(document.querySelector(".company-field").hidden).toBeTrue();
    expect(document.querySelector("#auth-form button").textContent).toContain("Login");
  });

  it("opens billing from the signed-in account state", function () {
    document.querySelector(".account-state").hidden = false;
    document.querySelector("#auth-form").hidden = true;
    document.querySelector("#account-name").textContent = "Test Co";
    document.querySelector("#account-email").textContent = "test@example.com";
    document.querySelector("#open-billing-button").click();
    expect(document.querySelector("#billing-dialog").open).toBeTrue();
    document.querySelector("#billing-dialog").close();
    expect(document.querySelector("#logout-button")).toBeNull();
  });

  it("has a full footer with product and company links", function () {
    var footer = document.querySelector(".site-footer");
    expect(footer).not.toBeNull();
    expect(footer.textContent).toContain("Product");
    expect(footer.textContent).toContain("Company");
    expect(footer.textContent).toContain("app.nz");
    expect(footer.textContent).toContain("lee101/gobed");
    expect(footer.textContent).not.toContain("hello@appliedai.nz");
    expect(footer.querySelectorAll("a").length).toBeGreaterThan(7);
  });

  it("keeps the trust section customer-facing", function () {
    var trust = document.querySelector("#trust");
    expect(trust).not.toBeNull();
    expect(trust.textContent).toContain("Give customers confidence");
    expect(trust.textContent).not.toContain("Postgres");
    expect(trust.textContent).not.toContain("Stripe Checkout");
    expect(trust.textContent).not.toContain("signed secure session cookies");
  });

  it("serves the health endpoint", function (done) {
    fetch("/health")
      .then(function (res) {
        expect(res.status).toBe(200);
        return res.json();
      })
      .then(function (body) {
        expect(body.ok).toBeTrue();
        done();
      })
      .catch(done.fail);
  });

  it("returns a controlled auth state from /api/me", function (done) {
    fetch("/api/me")
      .then(function (res) {
        expect([200, 401, 503]).toContain(res.status);
        return res.json();
      })
      .then(function (body) {
        expect(body.user || body.error).toBeDefined();
        done();
      })
      .catch(done.fail);
  });

  it("searches the customer directory API", function (done) {
    fetch("/api/customers?q=gobed")
      .then(function (res) {
        expect(res.status).toBe(200);
        return res.json();
      })
      .then(function (body) {
        expect(body.customers.length).toBeGreaterThan(0);
        expect(body.customers[0].domain).toContain("gobed");
        done();
      })
      .catch(done.fail);
  });

  it("exposes the status page API with controlled auth", function (done) {
    fetch("/api/v1/status-pages")
      .then(function (res) {
        expect([200, 401, 503]).toContain(res.status);
        return res.json();
      })
      .then(function (body) {
        expect(body.status_pages || body.error).toBeDefined();
        done();
      })
      .catch(done.fail);
  });
});

# TypeSafe Jev Playground

A high-performance, single-binary Go web application demonstrating the power of **Jev**, the new System One AI model by [TypeSafe.ai](https://typesafe.ai).

Unlike traditional LLMs that generate open-ended text, Jev is an inference engine designed purely for structured decision-making. It evaluates unstructured or complex JSON state against strictly typed questions in milliseconds, returning only programmable primitives: `Choices`, `Scores`, and `Nouls` (Booleans).

## How Jev Works

With standard Chat models (System Two), you write massive prompts and hope the model outputs parsable JSON. With Jev (System One), you define your schema explicitly.

1. **The State**: You pass the context (e.g., a raw JSON transaction payload, a user comment, or a customer support email).
2. **The Questions**: You pass a strict map of questions you want answered.
   * `Choice`: "Which department should this go to?" -> `[Billing, Tech, Sales]`
   * `Score`: "Rate the fraud risk severity." -> `[1 (Safe) to 5 (Critical)]`
   * `Noul`: "Is this an account takeover attempt?" -> `True / False`

The model evaluates the state and returns *only* those structured fields, alongside a `Confidence` score for each decision. This makes it incredibly fast, extremely cheap ($0.042 / 1M input tokens, $0 output), and fundamentally safe to wire directly into your application's `if/else` logic.

## Included Sandboxes

1. **Payment Risk Gateway**: Dynamically routes JSON transaction payloads (Amount, IP Distance, Device Fingerprint, Account Age) to detect Carding and Account Takeover (ATO) fraud without brittle manual rules.
2. **Support Ticket Triage**: Evaluates raw customer support emails to map urgency and route to the correct internal department.
3. **Content Moderation Engine**: Evaluates user-generated content for toxicity and policy violations in real-time.

## Local Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/shubhamkokul/jev-risk-router.git
   cd jev-risk-router
   ```
2. Set up your environment variables:
   ```bash
   cp .env.example .env
   ```
   Edit `.env` and add your `TYPESAFE_API_KEY`.
3. Run the server:
   ```bash
   go run main.go
   ```
4. Visit `http://localhost:8080`.

## Production Deployment & Security

This application is built as a single Go binary with no external assets (HTML/CSS/JS are embedded via HTMX and Tailwind CDN). It is strictly hardened for public deployment:
* **Max Payload Limits**: Rejects requests larger than 10 KB to prevent token exhaustion.
* **Input Truncation**: Slices text inputs to 1000 characters maximum.
* **Rate Limiting**: Built-in IP-based mutex throttling (1 request / 3 seconds / IP) honoring `X-Forwarded-For`.

To deploy on a Linux VPS:
```bash
GOOS=linux GOARCH=amd64 go build -o jev-router main.go
```
Run it via `systemd` and expose it behind a reverse proxy like Nginx or Caddy.

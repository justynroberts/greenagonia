#!/usr/bin/env bash
# ===========================================================================
# setup.sh — tiny CLI for the shared Greenagonia environment.
# ---------------------------------------------------------------------------
#   ./setup.sh setup                           interactive first-run wizard
#   ./setup.sh token                           set/replace the PagerDuty REST API token
#   ./setup.sh user-token                      set PagerDuty user-level token (Slack connections)
#   ./setup.sh slack-token                     set Slack bot token
#   ./setup.sh site-url [URL]                  set the storefront base URL
#   ./setup.sh admin add JR "Justyn Roberts" justyn@example.com
#   ./setup.sh admin remove JR
#   ./setup.sh admin list
#   ./setup.sh deploy                          terraform init + plan + apply
#   ./setup.sh urls [INITIALS]                 per-admin storefront links + keys
#   ./setup.sh destroy                         tear the whole environment down
#
# Config is stored as Terraform auto-loaded var files in this directory:
#   secrets.auto.tfvars.json   tokens          (chmod 600, gitignored)
#   config.auto.tfvars.json    admins, tz, site_url, flags  (gitignored)
# ===========================================================================
set -euo pipefail

cd "$(dirname "$0")"
SECRETS=secrets.auto.tfvars.json
CONFIG=config.auto.tfvars.json

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
ok()   { printf '\033[32m✓\033[0m %s\n' "$*"; }
err()  { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; }
die()  { err "$*"; exit 1; }

need() { command -v "$1" >/dev/null || die "$1 is required but not installed"; }
need python3
need terraform

json() { python3 - "$@"; } # helper: run inline python with args

# ---------------------------------------------------------------------------
ensure_config() {
  [ -f "$CONFIG" ] || cat > "$CONFIG" <<'EOF'
{
  "schedule_time_zone": "Europe/London",
  "site_url": "http://localhost:8080",
  "enable_incident_workflows": false,
  "enable_slack": false,
  "slack_workspace_id": "",
  "admins": {}
}
EOF
}

write_secrets() {
  # Merges key=value pairs into secrets.auto.tfvars.json.
  # Args: key1 val1 [key2 val2 …]
  python3 - "$@" <<'EOF'
import json, sys
pairs = dict(zip(sys.argv[1::2], sys.argv[2::2]))
try:
    data = json.load(open("secrets.auto.tfvars.json"))
except Exception:
    data = {}
data.update(pairs)
json.dump(data, open("secrets.auto.tfvars.json", "w"), indent=2)
EOF
  chmod 600 "$SECRETS"
  ok "saved to $SECRETS (chmod 600, gitignored)"
}

cmd_token() {
  printf 'PagerDuty REST API token (admin/write scope, input hidden): '
  read -rs TOKEN; echo
  [ -n "$TOKEN" ] || die "no token entered"
  printf 'Region [us/eu] (default us): '
  read -r REGION; REGION=${REGION:-us}
  [[ "$REGION" == "us" || "$REGION" == "eu" ]] || die "region must be us or eu"
  write_secrets pagerduty_token "$TOKEN" pagerduty_region "$REGION"
}

cmd_user_token() {
  printf 'PagerDuty user-level API token (My Profile → User Settings → Create API User Token, input hidden): '
  read -rs USER_TOKEN; echo
  [ -n "$USER_TOKEN" ] || die "no token entered"
  write_secrets pagerduty_user_token "$USER_TOKEN"
}

cmd_admin() {
  ensure_config
  case "${1:-}" in
    add)
      local initials="${2:-}" name="${3:-}" email="${4:-}" slack_email="${5:-}"
      [[ "$initials" =~ ^[A-Z]{2,4}$ ]] || die "usage: setup.sh admin add <INITIALS> <\"Full Name\"> <email> [slack-email]"
      [ -n "$name" ] && [ -n "$email" ] || die "usage: setup.sh admin add <INITIALS> <\"Full Name\"> <email> [slack-email]"
      [[ "$email" == *@*.* ]] || die "'$email' doesn't look like an email"
      [ -z "$slack_email" ] || [[ "$slack_email" == *@*.* ]] || die "'$slack_email' doesn't look like an email"
      python3 - "$initials" "$name" "$email" "$slack_email" <<'EOF'
import json, re, sys
cfg = json.load(open("config.auto.tfvars.json"))
admins = cfg["admins"]
ini, name, email, slack_email = sys.argv[1:5]

def entry(name, email, slack_email, existing=None):
    e = {"name": name, "email": email}
    if existing and existing.get("exists"):
        e["exists"] = True
    if slack_email:
        e["slack_email"] = slack_email
    return e

# Same email under the requested initials = same person, just update.
if ini in admins and admins[ini]["email"].lower() == email.lower():
    admins[ini] = entry(name, email, slack_email, admins[ini])
    json.dump(cfg, open("config.auto.tfvars.json", "w"), indent=2)
    suffix = f" (slack: {slack_email})" if slack_email else ""
    print(f"updated {ini} = {name} <{email}>{suffix}")
    raise SystemExit

# Initials taken by someone else → derive an alternative from the name.
if ini in admins:
    parts = [re.sub(r"[^A-Za-z]", "", p) for p in name.split() if re.sub(r"[^A-Za-z]", "", p)]
    first, last = (parts[0], parts[-1]) if len(parts) > 1 else (parts[0], parts[0])
    candidates = []
    for n in range(1, min(4, len(last)) + 1):
        candidates.append((first[0] + last[:n]).upper())
    for n in range(2, min(3, len(first)) + 1):
        candidates.append((first[:n] + last[0]).upper())
    candidates = [c for c in candidates if re.fullmatch(r"[A-Z]{2,4}", c)]
    pick = next((c for c in candidates if c not in admins), None)
    if pick is None:
        raise SystemExit(f"'{ini}' is taken and no free alternative could be derived from '{name}' — pass explicit initials")
    print(f"'{ini}' is taken by {admins[ini]['name']} — using {pick} instead")
    ini = pick

admins[ini] = entry(name, email, slack_email)
json.dump(cfg, open("config.auto.tfvars.json", "w"), indent=2)
suffix = f" (slack: {slack_email})" if slack_email else ""
print(f"added {ini} = {name} <{email}>{suffix}")
EOF
      echo "run './setup.sh deploy' to apply"
      ;;
    remove)
      local initials="${2:-}"
      [ -n "$initials" ] || die "usage: setup.sh admin remove <INITIALS>"
      python3 - "$initials" <<'EOF'
import json, sys
cfg = json.load(open("config.auto.tfvars.json"))
if cfg["admins"].pop(sys.argv[1], None) is None:
    raise SystemExit(f"no admin '{sys.argv[1]}' in config")
json.dump(cfg, open("config.auto.tfvars.json", "w"), indent=2)
print(f"removed {sys.argv[1]} — their PagerDuty stack will be DESTROYED on next deploy")
EOF
      echo "run './setup.sh deploy' to apply"
      ;;
    list|"")
      python3 - <<'EOF'
import json
cfg = json.load(open("config.auto.tfvars.json"))
admins = cfg["admins"]
if not admins:
    print("no admins configured — add one: ./setup.sh admin add JR \"Justyn Roberts\" justyn@example.com")
for ini, a in sorted(admins.items()):
    slack = f"  (slack: {a['slack_email']})" if a.get("slack_email") else ""
    print(f"  {ini:4} {a['name']} <{a['email']}>{slack}")
EOF
      ;;
    *) die "usage: setup.sh admin [add|remove|list]" ;;
  esac
}

cmd_setup() {
  bold "Greenagonia shared environment — setup"
  echo
  [ -f "$SECRETS" ] && ok "token already configured (re-run './setup.sh token' to replace)" || cmd_token
  ensure_config

  printf 'Schedule time zone (default Europe/London): '
  read -r TZONE; TZONE=${TZONE:-Europe/London}
  printf 'Storefront base URL (default http://localhost:8080): '
  read -r SURL; SURL=${SURL:-http://localhost:8080}
  printf 'Enable Incident Workflows? Paid feature, 404s without it [y/N]: '
  read -r WF
  printf 'Enable per-admin Slack incident channels? Needs Slack V2 integration + workflows [y/N]: '
  read -r SLACK
  SLACKWS=""
  if [[ "${SLACK:-n}" =~ ^[Yy] ]]; then
    printf 'Slack workspace ID (starts with T, from the PD Slack integration page): '
    read -r SLACKWS
    [[ "$SLACKWS" =~ ^T[A-Z0-9]+$ ]] || die "that doesn't look like a Slack workspace ID (e.g. T01ABCDEF23)"
  fi
  python3 - "$TZONE" "$SURL" "${WF:-n}" "${SLACK:-n}" "$SLACKWS" <<'EOF'
import json, sys
cfg = json.load(open("config.auto.tfvars.json"))
cfg["schedule_time_zone"] = sys.argv[1]
cfg["site_url"] = sys.argv[2].rstrip("/")
cfg["enable_incident_workflows"] = sys.argv[3].lower().startswith("y")
cfg["enable_slack"] = sys.argv[4].lower().startswith("y")
cfg["slack_workspace_id"] = sys.argv[5]
json.dump(cfg, open("config.auto.tfvars.json", "w"), indent=2)
EOF
  ok "settings saved to $CONFIG"

  if [ "$(python3 -c 'import json; print(len(json.load(open("config.auto.tfvars.json"))["admins"]))')" = "0" ]; then
    echo
    bold "Add the first admin"
    printf 'Initials (e.g. JR): '; read -r INI
    printf 'Full name: ';          read -r NAME
    printf 'Email (their real PagerDuty notification address): '; read -r EMAIL
    cmd_admin add "$INI" "$NAME" "$EMAIL"
  fi
  echo
  ok "setup complete"
  echo "next: ./setup.sh deploy"
}

cmd_deploy() {
  [ -f "$SECRETS" ] || die "no token configured — run './setup.sh setup' first"
  ensure_config
  terraform init -upgrade > /dev/null
  ok "terraform initialised"
  terraform plan -out=tfplan
  echo
  printf 'Apply this plan? [y/N]: '
  read -r YES
  [[ "${YES:-n}" =~ ^[Yy] ]] || { rm -f tfplan; die "aborted — nothing applied"; }
  terraform apply tfplan
  rm -f tfplan
  echo
  cmd_urls
}

cmd_urls() {
  local FILTER="${1:-}"
  python3 - "$FILTER" <<'EOF'
import json, subprocess, sys

filter_ini = sys.argv[1].upper() if len(sys.argv) > 1 and sys.argv[1] else None

def tf_output(name):
    try:
        r = subprocess.run(["terraform", "output", "-json", name],
                           capture_output=True, text=True, check=True)
        return json.loads(r.stdout)
    except Exception:
        return {}

routing      = tf_output("admin_routing_keys")
site_urls    = tf_output("admin_site_urls")
full_urls    = tf_output("admin_full_urls")
chg_keys     = tf_output("change_event_keys")
ld_keys      = tf_output("change_event_keys_ld")
platform_key = tf_output("platform_routing_key")

try:
    with open("config.auto.tfvars.json") as f:
        admins_cfg = json.load(f).get("admins", {})
except Exception:
    admins_cfg = {}

if not routing:
    sys.exit("no outputs yet — run ./setup.sh deploy")

if filter_ini and filter_ini not in routing and filter_ini != "PLATFORM":
    sys.exit(f"unknown admin '{filter_ini}' — known: {', '.join(sorted(routing))}, PLATFORM")

BOLD   = "\033[1m"
DIM    = "\033[2m"
GREEN  = "\033[32m"
CYAN   = "\033[36m"
YELLOW = "\033[33m"
RST    = "\033[0m"

# Shared platform section (shown when no filter or filter=PLATFORM)
if not filter_ini or filter_ini == "PLATFORM":
    if platform_key:
        print(f"\n{BOLD}PLATFORM{RST}  {DIM}(Greenagonia shared){RST}")
        print(f"  {DIM}routing key    {RST}{YELLOW}{platform_key}{RST}")
        print(f"  {DIM}services       {RST}api-gateway · data-platform · identity-service · infrastructure · platform-engineering")

for ini in sorted(routing):
    if filter_ini and ini != filter_ini:
        continue
    rk = routing[ini]
    cfg = admins_cfg.get(ini, {})
    label = f"  {DIM}({cfg['name']} · {cfg['email']}){RST}" if cfg else ""
    print(f"\n{BOLD}{ini}{RST}{label}")
    print(f"  {DIM}routing key    {RST}{GREEN}{rk}{RST}")
    print(f"  {DIM}storefront     {RST}{CYAN}{site_urls.get(ini, '')}{RST}")
    full = full_urls.get(ini, "")
    if full:
        print(f"  {DIM}full URL       {RST}{CYAN}{full}{RST}")

    # Change keys — show the GitHub key for each service
    svc_keys = {k.split("/", 1)[1]: v
                for k, v in chg_keys.items() if k.startswith(ini + "/")}
    if svc_keys:
        print(f"\n  {BOLD}Change event keys{RST}")
        for svc, key in sorted(svc_keys.items()):
            label = "github"
            ld_key_entry = next(
                (v for k, v in ld_keys.items() if k.startswith(ini + "/")), None
            ) if svc == "payment-gateway" else None
            print(f"  {DIM}  {svc:<28}{RST}{key}  {DIM}({label}){RST}")
            if ld_key_entry:
                print(f"  {DIM}  {'':28}{RST}{ld_key_entry}  {DIM}(launchdarkly){RST}")
EOF
}

cmd_site_url() {
  ensure_config
  local URL="${1:-}"
  if [ -z "$URL" ]; then
    printf 'Storefront base URL (e.g. http://3.85.144.140 or https://demo.example.com): '
    read -r URL
  fi
  [ -n "$URL" ] || die "no URL entered"
  python3 - "$URL" <<'EOF'
import json, sys
cfg = json.load(open("config.auto.tfvars.json"))
cfg["site_url"] = sys.argv[1].rstrip("/")
json.dump(cfg, open("config.auto.tfvars.json", "w"), indent=2)
print(f"site_url = {cfg['site_url']}")
EOF
  ok "saved — run ./setup.sh deploy to apply, then ./setup.sh urls to see new links"
}

cmd_slack_token() {
  printf 'Slack bot token (xoxb-…, input hidden): '
  read -rs SLACK_TOKEN; echo
  [ -n "$SLACK_TOKEN" ] || die "no token entered"
  [[ "$SLACK_TOKEN" == xoxb-* ]] || die "token must start with xoxb-"
  write_secrets slack_bot_token "$SLACK_TOKEN"
  ok "Run ./setup.sh deploy to create the Slack channels."
}

cmd_slack_channels() {
  [ -f slack-secrets.json ] || die "no Slack credentials — run ./setup.sh slack-token first"
  [ -f "$CONFIG" ] || die "no config found — run ./setup.sh setup first"
  python3 - <<'EOF'
import json, urllib.request, urllib.parse, sys

creds   = json.load(open("slack-secrets.json"))
cfg     = json.load(open("config.auto.tfvars.json"))
TOKEN   = creds["slack_bot_token"]
admins  = cfg.get("admins", {})

BOLD  = "\033[1m"
DIM   = "\033[2m"
GREEN = "\033[32m"
RED   = "\033[31m"
RST   = "\033[0m"

def slack(method, path, **data):
    url = f"https://slack.com/api/{path}"
    if method == "GET":
        url += "?" + urllib.parse.urlencode(data)
        req = urllib.request.Request(url)
    else:
        req = urllib.request.Request(url, json.dumps(data).encode(),
                                     {"Content-Type": "application/json"})
    req.add_header("Authorization", f"Bearer {TOKEN}")
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read())

errors = []
for ini, admin in sorted(admins.items()):
    channel_name = f"{ini.lower()}-incidents"
    email        = admin["email"]

    # 1. Create the channel (idempotent — name_taken is fine)
    r = slack("POST", "conversations.create", name=channel_name, is_private=False)
    if r.get("ok"):
        channel_id = r["channel"]["id"]
        print(f"{GREEN}✓{RST} {BOLD}{channel_name}{RST} created ({channel_id})")
    elif r.get("error") == "name_taken":
        # Look it up
        r2 = slack("GET", "conversations.list", types="public_channel", limit=200)
        ch = next((c for c in r2.get("channels", []) if c["name"] == channel_name), None)
        if not ch:
            # Try private
            r2 = slack("GET", "conversations.list", types="private_channel", limit=200)
            ch = next((c for c in r2.get("channels", []) if c["name"] == channel_name), None)
        if ch:
            channel_id = ch["id"]
            print(f"{DIM}–{RST} {BOLD}{channel_name}{RST} already exists ({channel_id})")
        else:
            print(f"{RED}✗{RST} {channel_name}: created but couldn't look up ID")
            errors.append(channel_name)
            continue
    else:
        print(f"{RED}✗{RST} {channel_name}: {r.get('error', 'unknown error')}")
        errors.append(channel_name)
        continue

    # 2. Look up the user by email
    r = slack("GET", "users.lookupByEmail", email=email)
    if not r.get("ok"):
        print(f"  {RED}✗{RST} user {email}: {r.get('error', 'not found')} — skipping invite")
        errors.append(f"{ini} user lookup")
        continue
    user_id = r["user"]["id"]

    # 3. Invite the user (already_in_channel is fine)
    r = slack("POST", "conversations.invite", channel=channel_id, users=user_id)
    if r.get("ok"):
        print(f"  {GREEN}✓{RST} invited {email} ({user_id})")
    elif r.get("error") in ("already_in_channel", "cant_invite_self"):
        print(f"  {DIM}–{RST} {email} already in channel")
    else:
        print(f"  {RED}✗{RST} invite {email}: {r.get('error', 'unknown')}")
        errors.append(f"{ini} invite")

if errors:
    print(f"\n{RED}Completed with errors:{RST} {', '.join(errors)}")
    sys.exit(1)
else:
    print(f"\n{GREEN}All channels ready.{RST}")
EOF
}

cmd_destroy() {
  bold "This DESTROYS the entire shared environment in PagerDuty:"
  echo "  every admin's team, services, schedules, orchestration — and the personas."
  printf 'Type "destroy" to confirm: '
  read -r CONFIRM
  [ "$CONFIRM" = "destroy" ] || die "aborted"
  terraform destroy -auto-approve
}

case "${1:-help}" in
  setup)          cmd_setup ;;
  token)          cmd_token ;;
  user-token)     cmd_user_token ;;
  admin)          shift; cmd_admin "$@" ;;
  deploy)         cmd_deploy ;;
  urls)           shift; cmd_urls "$@" ;;
  site-url)       shift; cmd_site_url "$@" ;;
  slack-token)    cmd_slack_token ;;
  slack-channels) cmd_slack_channels ;;
  destroy)        cmd_destroy ;;
  *)
    cat <<'EOF'
Greenagonia shared environment — setup CLI

  ./setup.sh setup                                     first-run wizard (token, settings, first admin)
  ./setup.sh token                                     set/replace the PagerDuty REST API token
  ./setup.sh user-token                               set/replace the PagerDuty user-level token (needed for Slack connections)
  ./setup.sh admin add JR "Justyn Roberts" jr@x.com [slack@x.com]   add/update an admin (slack email optional if different)
  ./setup.sh admin remove JR                           remove an admin (stack destroyed on next deploy)
  ./setup.sh admin list                                show configured admins
  ./setup.sh deploy                                    terraform init + plan + confirm + apply
  ./setup.sh urls [INITIALS]                           per-admin storefront links with keys (e.g. urls JP)
  ./setup.sh site-url [URL]                            set the storefront base URL (e.g. http://3.85.144.140)
  ./setup.sh slack-token                               save Slack bot token + workspace ID
  ./setup.sh slack-channels                            create {ini}-incidents channels and invite each admin
  ./setup.sh destroy                                   tear everything down
EOF
    ;;
esac

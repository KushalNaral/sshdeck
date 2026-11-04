# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack
Wails v2 desktop shell (Go backend) with a React + TypeScript + Tailwind frontend and xterm.js for terminals. Linux first.

## Users
A PHP/web-hosting support engineer and a few teammates on the same support team. Each runs their own copy with their own OS keyring and imports the same server list. They work in a bright office by day and in dim rooms late at night during incidents, so the theme follows the OS.

## Product Purpose
sshdeck replaces a spreadsheet of plain-text server passwords (about 20 servers behind about 42 PHP client projects, several behind FortiClient, GlobalProtect or Hillstone VPNs). It stores credentials safely, brings up the right VPN, opens SSH terminals, and runs read-only analysis of each VPS and its containers. Success: from "a client site is down" to a working shell and a clear picture of the box in seconds, and a fleet-wide PHP patch campaign that can be tracked server by server.

## Positioning
A purpose-built desk for one team's fixed estate: it knows which project lives on which server, which VPN gates it, and what PHP version it should be patched to. A generic SSH client or admin dashboard does not know any of that.

## Operating Context
Two jobs, about equally often:
1. Firefighting: pick a server (or a project, which maps to a server), Connect = VPN first if needed then SSH, check containers, failing services, disk and memory, fix.
2. PHP patch campaign: scan the fleet for servers below their patched PHP target, patch, verify, track compatibility status.
Source data is the ODS workbook `PHP MASTER PROJECT LIST (1).ods`: a `main_screen` sheet (Project, Type, Current versions OS/PHP/framework, Patched version, Compatibility, Server, Project status, Contact) plus one irregular sheet per server.

## Capabilities and Constraints
- SSH terminals (multi-tab), read-only analysis (OS, PHP, web server, DB, disk, memory, failed services, Docker containers and compose).
- VPN control via Linux CLI clients: openfortivpn (Forti), openconnect --protocol=gp (GlobalProtect). Hillstone has no Linux CLI, so it is a manual-step reminder. VPN needs root via pkexec.
- Passwords live in the OS keyring, never in plain files. RDP, cPanel and Jenkins entries are stored notes/links with copy buttons, with no built-in client.
- Analysis never writes to a server.
- Undecided: Tundra's VPN type (sheet says "VPN Required" without details).

## Evidence on Hand
Real server and project names exist in the ODS; use them for demonstration data. Do not invent credentials, IPs or versions in shipped UI; synthetic demo data must be labelled.

## Product Principles
1. The job is a flow, not a menu: Connect does VPN then SSH then analysis without the user sequencing it.
2. Connection state (VPN up, SSH live, why it failed) is always visible and never ambiguous.
3. The terminal is the product; chrome gives way to it.
4. Fast to switch: servers, tabs and views change instantly.
5. The fleet view answers "what still needs patching" at a glance.

## Accessibility & Inclusion
Readable in both bright and dim rooms; state is never conveyed by color alone; full keyboard operation (Ctrl+K server search, Ctrl+T new terminal).

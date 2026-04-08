# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| main (latest) | Yes |
| Older branches | No |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security bugs.**

Email [security@anthropic.com](mailto:security@anthropic.com) with subject:
**[claude-code-go] Security Vulnerability Report**

Include:
- Description of the vulnerability and its impact
- Steps to reproduce
- Proof-of-concept code (if applicable)
- Suggested severity (critical / high / medium / low)

You will receive:
- Acknowledgement within 3 business days
- Status update within 10 business days
- Coordinated public disclosure

## Scope

In scope:
- Authentication bypass (OAuth, API key handling)
- Remote code execution via tool inputs or MCP config
- Data exfiltration through tool use or network requests
- API key / secret leakage in logs, state files, or traffic

Out of scope:
- Vulnerabilities in third-party dependencies (report upstream)
- Issues requiring physical machine access
- Theoretical vulnerabilities without proof of concept

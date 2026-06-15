# ZenGate AI - Self-Healer Module

[![Build Status](https://img.shields.io/badge/build-passing-brightgreen)]()
[![License](https://img.shields.io/badge/license-MIT-blue)]()
[![Go Version](https://img.shields.io/badge/go-1.21+-blue)]()

## Overview
The Self-Healer module is an autonomous diagnostic agent designed to monitor system health and execute repair plans using LLM-based reasoning.

## Model Configuration
The system currently utilizes `gemini-1.5-flash` for its high-performance, low-latency capabilities within the Google AI Studio free tier.

### Environment Variables
| Variable | Description | Default |
| :--- | :--- | :--- |
| `GEMINI_API_KEY` | Your Google AI Studio API Key | Required |
| `GEMINI_MODEL` | The model identifier to use | `gemini-1.5-flash` |

## Architecture
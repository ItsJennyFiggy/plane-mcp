#!/usr/bin/env bats
#
# Unit tests for .devcontainer/agent/devcontainer.json (PLAT-3).
# Tests follow Arrange-Act-Assert. All checks are pure filesystem/JSON reads —
# no Docker daemon, network, or AWS calls are made.

AGENT_DC="${BATS_TEST_DIRNAME}/../.devcontainer/agent/devcontainer.json"
HUMAN_DC="${BATS_TEST_DIRNAME}/../.devcontainer/devcontainer.json"

# ---------------------------------------------------------------------------
# Structural validity
# ---------------------------------------------------------------------------

@test "agent devcontainer.json is valid JSON" {
  # Arrange — file must exist
  [ -f "$AGENT_DC" ]
  # Act
  run python3 -c "import sys, json; json.load(open(sys.argv[1]))" "$AGENT_DC"
  # Assert
  [ "${status}" -eq 0 ]
}

@test "human devcontainer.json is valid JSON" {
  # Arrange
  [ -f "$HUMAN_DC" ]
  # Act
  run python3 -c "import sys, json; json.load(open(sys.argv[1]))" "$HUMAN_DC"
  # Assert
  [ "${status}" -eq 0 ]
}

# ---------------------------------------------------------------------------
# Agent persona: required security properties
# ---------------------------------------------------------------------------

@test "agent devcontainer.json sets AGENT_SHELL=1 in containerEnv" {
  # Arrange
  [ -f "$AGENT_DC" ]
  # Act — extract the AGENT_SHELL value from containerEnv
  run python3 -c "
import sys, json
doc = json.load(open(sys.argv[1]))
val = doc.get('containerEnv', {}).get('AGENT_SHELL', '')
print(val)
" "$AGENT_DC"
  # Assert
  [ "${status}" -eq 0 ]
  [ "${output}" = "1" ]
}

@test "agent devcontainer.json mounts ~/.aws read-only" {
  # Arrange
  [ -f "$AGENT_DC" ]
  # Act — check that at least one mount contains both 'aws' and 'readonly'
  run python3 -c "
import sys, json
doc = json.load(open(sys.argv[1]))
mounts = doc.get('mounts', [])
ro_aws = [m for m in mounts if '.aws' in m and 'readonly' in m]
print(len(ro_aws))
" "$AGENT_DC"
  # Assert
  [ "${status}" -eq 0 ]
  [ "${output}" = "1" ]
}

@test "agent devcontainer.json does not mount ~/.ssh or personal SSH keys" {
  # Arrange
  [ -f "$AGENT_DC" ]
  # Act — confirm no .ssh mount exists
  run python3 -c "
import sys, json
doc = json.load(open(sys.argv[1]))
mounts = doc.get('mounts', [])
ssh_mounts = [m for m in mounts if '.ssh' in m]
print(len(ssh_mounts))
" "$AGENT_DC"
  # Assert
  [ "${status}" -eq 0 ]
  [ "${output}" = "0" ]
}

@test "agent devcontainer.json postStartCommand prepends .agents/bin to PATH" {
  # Arrange
  [ -f "$AGENT_DC" ]
  # Act
  run python3 -c "
import sys, json
doc = json.load(open(sys.argv[1]))
cmd = doc.get('postStartCommand', '')
print('yes' if '.agents/bin' in cmd else 'no')
" "$AGENT_DC"
  # Assert
  [ "${status}" -eq 0 ]
  [ "${output}" = "yes" ]
}

@test "agent devcontainer.json uses the expected base image" {
  # Arrange
  [ -f "$AGENT_DC" ]
  # Act
  run python3 -c "
import sys, json
doc = json.load(open(sys.argv[1]))
image = doc.get('image', '')
print(image)
" "$AGENT_DC"
  # Assert
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"mcr.microsoft.com/devcontainers/base"* ]]
}

# ---------------------------------------------------------------------------
# Human persona: mounts present, read-only flag absent
# ---------------------------------------------------------------------------

@test "human devcontainer.json mounts ~/.aws without readonly restriction" {
  # Arrange
  [ -f "$HUMAN_DC" ]
  # Act — confirm .aws is mounted and readonly is NOT set
  run python3 -c "
import sys, json
doc = json.load(open(sys.argv[1]))
mounts = doc.get('mounts', [])
aws_mounts      = [m for m in mounts if '.aws' in m]
ro_aws_mounts   = [m for m in aws_mounts if 'readonly' in m]
print(len(aws_mounts), len(ro_aws_mounts))
" "$HUMAN_DC"
  # Assert: at least 1 .aws mount, 0 readonly .aws mounts
  [ "${status}" -eq 0 ]
  [ "${output}" = "1 0" ]
}

@test "human devcontainer.json mounts ~/.gitconfig" {
  # Arrange
  [ -f "$HUMAN_DC" ]
  # Act
  run python3 -c "
import sys, json
doc = json.load(open(sys.argv[1]))
mounts = doc.get('mounts', [])
git_mounts = [m for m in mounts if '.gitconfig' in m]
print(len(git_mounts))
" "$HUMAN_DC"
  # Assert
  [ "${status}" -eq 0 ]
  [ "${output}" = "1" ]
}

@test "human devcontainer.json does not set AGENT_SHELL in containerEnv" {
  # Arrange
  [ -f "$HUMAN_DC" ]
  # Act
  run python3 -c "
import sys, json
doc = json.load(open(sys.argv[1]))
val = doc.get('containerEnv', {}).get('AGENT_SHELL', 'unset')
print(val)
" "$HUMAN_DC"
  # Assert — AGENT_SHELL must be absent from the human persona
  [ "${status}" -eq 0 ]
  [ "${output}" = "unset" ]
}

# ---------------------------------------------------------------------------
# Persona separation: agent must differ from human on key axes
# ---------------------------------------------------------------------------

@test "agent and human personas use the same base image" {
  # Arrange
  [ -f "$AGENT_DC" ] && [ -f "$HUMAN_DC" ]
  # Act
  run python3 -c "
import sys, json
agent = json.load(open(sys.argv[1])).get('image', '')
human = json.load(open(sys.argv[2])).get('image', '')
print('same' if agent == human else 'different')
" "$AGENT_DC" "$HUMAN_DC"
  # Assert — keeping images in sync reduces drift
  [ "${status}" -eq 0 ]
  [ "${output}" = "same" ]
}

#!/usr/bin/env bats
#
# Unit tests for .agents/bin/terraform — the apply/destroy intercept wrapper.
# Tests follow Arrange-Act-Assert. The real terraform binary is mocked via a
# stub placed earlier in PATH; no network or AWS calls are made.

WRAPPER="${BATS_TEST_DIRNAME}/../.agents/bin/terraform"

setup() {
  # Create a per-test temp dir for the stub terraform binary.
  STUB_DIR="${BATS_TEST_TMPDIR}/stub_bin"
  mkdir -p "$STUB_DIR"

  # Stub: echoes its arguments so we can assert pass-through, then exits 0.
  cat > "${STUB_DIR}/terraform" <<'STUB'
#!/usr/bin/env bash
echo "real-terraform $*"
exit 0
STUB
  chmod +x "${STUB_DIR}/terraform"

  # Export so the wrapper can locate the stub via command -v -p / candidate loop.
  export PATH="${STUB_DIR}:${PATH}"
}

# ---------------------------------------------------------------------------
# Blocked commands (AGENT_SHELL=1)
# ---------------------------------------------------------------------------

@test "terraform apply is blocked in agent shell" {
  # Arrange
  export AGENT_SHELL=1
  # Act
  run bash "${WRAPPER}" apply
  # Assert
  [ "${status}" -eq 1 ]
  [[ "${output}" == *"BLOCKED"* ]]
}

@test "terraform destroy is blocked in agent shell" {
  # Arrange
  export AGENT_SHELL=1
  # Act
  run bash "${WRAPPER}" destroy
  # Assert
  [ "${status}" -eq 1 ]
  [[ "${output}" == *"BLOCKED"* ]]
}

@test "terraform -chdir=infra/global apply is blocked in agent shell" {
  # Arrange — flag precedes the subcommand; wrapper must skip the flag and
  # detect apply as the first non-flag argument.
  export AGENT_SHELL=1
  # Act
  run bash "${WRAPPER}" -chdir=infra/global apply
  # Assert
  [ "${status}" -eq 1 ]
  [[ "${output}" == *"BLOCKED"* ]]
}

# ---------------------------------------------------------------------------
# Allowed commands (AGENT_SHELL=1)
# ---------------------------------------------------------------------------

@test "terraform plan is allowed in agent shell" {
  # Arrange
  export AGENT_SHELL=1
  # Act
  run bash "${WRAPPER}" plan
  # Assert
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"real-terraform"* ]]
}

@test "terraform plan -out=apply.tfplan is allowed in agent shell" {
  # Arrange — the word 'apply' appears only inside a flag value, not as a
  # subcommand; wrapper must NOT block this.
  export AGENT_SHELL=1
  # Act
  run bash "${WRAPPER}" plan -out=apply.tfplan
  # Assert
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"real-terraform"* ]]
}

# ---------------------------------------------------------------------------
# Pass-through when AGENT_SHELL is unset
# ---------------------------------------------------------------------------

@test "terraform apply passes through when AGENT_SHELL is unset" {
  # Arrange
  unset AGENT_SHELL
  # Act
  run bash "${WRAPPER}" apply
  # Assert
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"real-terraform"* ]]
}

@test "terraform destroy passes through when AGENT_SHELL is unset" {
  # Arrange
  unset AGENT_SHELL
  # Act
  run bash "${WRAPPER}" destroy
  # Assert
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"real-terraform"* ]]
}

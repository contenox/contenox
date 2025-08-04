import requests
from unittest.mock import patch, MagicMock
import pytest

class TestHookExecution:
    """Test cases for verifying hook execution through the API"""
    @pytest.fixture
    def api_base_url(self):
        """Base URL for the Go service API"""
        return "http://localhost:8080/api/v1"

    @patch('requests.post')
    def test_remote_hook_execution_success(self, mock_post, api_base_url):
        """Test successful execution of a remote hook"""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "response": "Remote operation completed",
            "dataType": "string",
            "transition": "next_task",
            "state": [{  # Execution history
                "taskID": "hook_task",
                "output": "Remote operation completed"
            }]
        }
        mock_post.return_value = mock_response

        # Execute a chain that uses a remote hook
        chain_execution = {
            "chain_id": "document-processing",
            "input": "Test document content",
            "data_type": "string"
        }

        response = requests.post(
            f"{api_base_url}/chains/execute",
            json=chain_execution
        )

        # Verify the chain execution response
        assert response.status_code == 200
        result = response.json()

        # The API returns "response" field, NOT "output"
        assert result.get("response") == "Remote operation completed"
        assert result.get("dataType") == "string"
        assert any(step.get("output") == "Remote operation completed"
                  for step in result.get("state", []))

    @patch('requests.post')
    def test_remote_hook_failure_handling(self, mock_post, api_base_url):
        """Test proper error handling when remote hook returns error"""
        # Mock the response for a failed remote hook call
        mock_response = MagicMock()
        mock_response.status_code = 500
        mock_response.json.return_value = {
            "error": "remote hook returned status: 500",  # Error field name
            "state": [{
                "taskID": "hook_task",
                "error": {"Error": "remote hook returned status: 500"}
            }]
        }
        mock_post.return_value = mock_response

        # Execute a chain with a remote hook
        response = requests.post(
            f"{api_base_url}/chains/execute",
            json={
                "chain_id": "error-chain",
                "input": "Test input",
                "data_type": "string"
            }
        )

        assert response.status_code == 500
        result = response.json()

        # Check if the error message contains what we expect
        error_msg = result.get("error", "")
        assert "remote hook returned status: 500" in error_msg.lower()

        # Also check the state for error details
        if "state" in result and len(result["state"]) > 0:
            task_error = result["state"][0].get("error", {})
            if isinstance(task_error, dict):
                error_desc = task_error.get("Error", "")
                assert "remote hook returned status: 500" in error_desc.lower()

    @patch('requests.post')
    def test_remote_hook_with_timeout(self, mock_post, api_base_url):
        """Test remote hook execution with timeout handling"""
        # Mock timeout response
        mock_response = MagicMock()
        mock_response.status_code = 500
        mock_response.json.return_value = {
            "error": "remote hook request failed: context deadline exceeded",
            "state": [{
                "taskID": "timeout-task",
                "error": {"Error": "remote hook request failed: context deadline exceeded"}
            }]
        }
        mock_post.return_value = mock_response

        # Execute a chain with a remote hook
        response = requests.post(
            f"{api_base_url}/chains/execute",
            json={
                "chain_id": "timeout-test-chain",
                "input": "Test input",
                "data_type": "string"
            }
        )

        assert response.status_code == 500
        result = response.json()
        error_msg = result.get("error", "")
        assert "deadline exceeded" in error_msg.lower() or "timeout" in error_msg.lower()

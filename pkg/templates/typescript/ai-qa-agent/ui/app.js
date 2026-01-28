// Form submission handler
document.getElementById('qaForm').addEventListener('submit', async (e) => {
  e.preventDefault();

  const submitBtn = document.getElementById('submitBtn');
  const resultsPanel = document.getElementById('resultsPanel');
  const errorPanel = document.getElementById('errorPanel');
  const progressPanel = document.getElementById('progressPanel');
  const progressSteps = document.getElementById('progressSteps');

  // Disable submit button and show loading
  submitBtn.disabled = true;
  submitBtn.querySelector('.btn-text').style.display = 'none';
  submitBtn.querySelector('.btn-loader').style.display = 'inline-flex';

  // Hide previous results/errors, show progress
  resultsPanel.style.display = 'none';
  errorPanel.style.display = 'none';
  progressPanel.style.display = 'block';
  progressSteps.innerHTML = '<div class="progress-step active"><span class="spinner"></span> Initializing...</div>';

  try {
    // Collect form data
    const formData = new FormData(e.target);
    const payload = buildPayload(formData);

    console.log('Sending payload:', payload);

    // Send request to server to get session ID
    const response = await fetch('/api/run-qa', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || 'Analysis failed');
    }

    const { sessionId } = await response.json();

    // Connect to SSE for progress updates
    const eventSource = new EventSource(`/api/progress/${sessionId}`);

    eventSource.onmessage = (event) => {
      const data = JSON.parse(event.data);

      if (data.type === 'status') {
        addProgressStep(data.step, data.message);
      } else if (data.type === 'complete') {
        eventSource.close();
        progressPanel.style.display = 'none';
        displayResults(data.result);
        resultsPanel.style.display = 'block';
        resultsPanel.scrollIntoView({ behavior: 'smooth', block: 'nearest' });

        // Re-enable submit button
        submitBtn.disabled = false;
        submitBtn.querySelector('.btn-text').style.display = 'inline';
        submitBtn.querySelector('.btn-loader').style.display = 'none';
      } else if (data.type === 'error') {
        eventSource.close();
        // Handle error properly instead of throwing
        progressPanel.style.display = 'none';
        document.getElementById('errorMessage').textContent = data.error || 'An unknown error occurred';
        errorPanel.style.display = 'block';
        errorPanel.scrollIntoView({ behavior: 'smooth', block: 'nearest' });

        // Re-enable submit button
        submitBtn.disabled = false;
        submitBtn.querySelector('.btn-text').style.display = 'inline';
        submitBtn.querySelector('.btn-loader').style.display = 'none';
      }
    };

    eventSource.onerror = (error) => {
      console.error('SSE Error:', error);
      eventSource.close();
      
      // Handle SSE connection errors
      progressPanel.style.display = 'none';
      document.getElementById('errorMessage').textContent = 'Connection to server lost. Please try again.';
      errorPanel.style.display = 'block';

      // Re-enable submit button
      submitBtn.disabled = false;
      submitBtn.querySelector('.btn-text').style.display = 'inline';
      submitBtn.querySelector('.btn-loader').style.display = 'none';
    };

  } catch (error) {
    console.error('Error:', error);
    progressPanel.style.display = 'none';
    document.getElementById('errorMessage').textContent = error.message;
    errorPanel.style.display = 'block';
    errorPanel.scrollIntoView({ behavior: 'smooth', block: 'nearest' });

    // Re-enable submit button
    submitBtn.disabled = false;
    submitBtn.querySelector('.btn-text').style.display = 'inline';
    submitBtn.querySelector('.btn-loader').style.display = 'none';
  }
});

// Add progress step to UI
function addProgressStep(step, message) {
  const progressSteps = document.getElementById('progressSteps');

  // Mark previous step as complete
  const activeSteps = progressSteps.querySelectorAll('.progress-step.active');
  activeSteps.forEach(step => {
    step.classList.remove('active');
    step.classList.add('complete');
    step.querySelector('.spinner')?.remove();
    const checkmark = document.createElement('span');
    checkmark.className = 'checkmark';
    checkmark.textContent = '✓';
    step.insertBefore(checkmark, step.firstChild);
  });

  // Add new step
  const stepDiv = document.createElement('div');
  stepDiv.className = 'progress-step active';
  stepDiv.innerHTML = `<span class="spinner"></span> ${escapeHtml(message)}`;
  progressSteps.appendChild(stepDiv);

  // Auto-scroll to bottom
  progressSteps.scrollTop = progressSteps.scrollHeight;
}

// Build payload from form data
function buildPayload(formData) {
  const payload = {
    url: formData.get('url'),
    model: formData.get('model'),
    checks: {},
    context: {},
  };

  // Compliance checks
  const compliance = {};
  if (formData.get('compliance.accessibility')) compliance.accessibility = true;
  if (formData.get('compliance.legal')) compliance.legal = true;
  if (formData.get('compliance.brand')) compliance.brand = true;
  if (formData.get('compliance.regulatory')) compliance.regulatory = true;
  if (Object.keys(compliance).length > 0) {
    payload.checks.compliance = compliance;
  }

  // Policy violations
  const policyViolations = {};
  if (formData.get('policyViolations.content')) policyViolations.content = true;
  if (formData.get('policyViolations.security')) policyViolations.security = true;
  if (Object.keys(policyViolations).length > 0) {
    payload.checks.policyViolations = policyViolations;
  }

  // Broken UI
  if (formData.get('brokenUI')) {
    payload.checks.brokenUI = true;
  }

  // Dismiss Popups
  if (formData.get('dismissPopups')) {
    payload.dismissPopups = true;
  }

  // Context
  const industry = formData.get('context.industry');
  if (industry) payload.context.industry = industry;

  const brandGuidelines = formData.get('context.brandGuidelines');
  if (brandGuidelines && brandGuidelines.trim()) {
    payload.context.brandGuidelines = brandGuidelines.trim();
  }

  const customPolicies = formData.get('context.customPolicies');
  if (customPolicies && customPolicies.trim()) {
    payload.context.customPolicies = customPolicies.trim();
  }

  return payload;
}

// Display results
function displayResults(result) {
  const container = document.getElementById('resultsContent');

  // Create summary cards
  const summary = result.summary;
  const summaryHTML = `
    <div class="summary-cards">
      <div class="summary-card total">
        <div class="number">${summary.totalIssues}</div>
        <div class="label">Total Issues</div>
      </div>
      <div class="summary-card critical">
        <div class="number">${summary.criticalIssues}</div>
        <div class="label">Critical</div>
      </div>
      <div class="summary-card warning">
        <div class="number">${summary.warnings}</div>
        <div class="label">Warnings</div>
      </div>
      <div class="summary-card info">
        <div class="number">${summary.infos}</div>
        <div class="label">Info</div>
      </div>
    </div>
  `;

  // Group issues by category
  const issues = result.issues;
  const complianceIssues = issues.filter(i => i.category === 'compliance');
  const policyIssues = issues.filter(i => i.category === 'policy');
  const uiIssues = issues.filter(i => i.category === 'visual');

  let issuesHTML = '';

  // Compliance Issues
  if (complianceIssues.length > 0) {
    const byType = {
      accessibility: complianceIssues.filter(i => i.complianceType === 'accessibility'),
      legal: complianceIssues.filter(i => i.complianceType === 'legal'),
      brand: complianceIssues.filter(i => i.complianceType === 'brand'),
      regulatory: complianceIssues.filter(i => i.complianceType === 'regulatory'),
    };

    issuesHTML += '<div class="issue-section"><h3>Compliance Issues (' + complianceIssues.length + ')</h3>';

    if (byType.accessibility.length > 0) {
      issuesHTML += '<h4 style="color: #7f8c8d; font-size: 1.1rem; margin: 15px 0 10px 0;">Accessibility</h4>';
      issuesHTML += byType.accessibility.map(renderIssue).join('');
    }
    if (byType.legal.length > 0) {
      issuesHTML += '<h4 style="color: #7f8c8d; font-size: 1.1rem; margin: 15px 0 10px 0;">Legal</h4>';
      issuesHTML += byType.legal.map(renderIssue).join('');
    }
    if (byType.brand.length > 0) {
      issuesHTML += '<h4 style="color: #7f8c8d; font-size: 1.1rem; margin: 15px 0 10px 0;">Brand</h4>';
      issuesHTML += byType.brand.map(renderIssue).join('');
    }
    if (byType.regulatory.length > 0) {
      issuesHTML += '<h4 style="color: #7f8c8d; font-size: 1.1rem; margin: 15px 0 10px 0;">Regulatory</h4>';
      issuesHTML += byType.regulatory.map(renderIssue).join('');
    }

    issuesHTML += '</div>';
  }

  // Policy Violations
  if (policyIssues.length > 0) {
    const byType = {
      content: policyIssues.filter(i => i.violationType === 'content'),
      security: policyIssues.filter(i => i.violationType === 'security'),
    };

    issuesHTML += '<div class="issue-section"><h3>Policy Violations (' + policyIssues.length + ')</h3>';

    if (byType.content.length > 0) {
      issuesHTML += '<h4 style="color: #7f8c8d; font-size: 1.1rem; margin: 15px 0 10px 0;">Content Policy</h4>';
      issuesHTML += byType.content.map(renderIssue).join('');
    }
    if (byType.security.length > 0) {
      issuesHTML += '<h4 style="color: #7f8c8d; font-size: 1.1rem; margin: 15px 0 10px 0;">Security</h4>';
      issuesHTML += byType.security.map(renderIssue).join('');
    }

    issuesHTML += '</div>';
  }

  // UI Issues
  if (uiIssues.length > 0) {
    issuesHTML += '<div class="issue-section"><h3>Broken UI Issues (' + uiIssues.length + ')</h3>';
    issuesHTML += uiIssues.map(renderIssue).join('');
    issuesHTML += '</div>';
  }

  if (issues.length === 0) {
    issuesHTML = '<div style="text-align: center; padding: 48px 32px; color: #10B981; font-size: 14px; background: rgba(16, 185, 129, 0.1); border-radius: 8px; border: 1px solid rgba(16, 185, 129, 0.2);"><p style="font-size: 32px; margin-bottom: 8px;">✓</p>No issues found! The website passed all checks.</div>';
  }

  container.innerHTML = summaryHTML + issuesHTML;

  // Store HTML report for export
  window.currentHtmlReport = result.htmlReport;
}

// Render individual issue
function renderIssue(issue) {
  let badges = `<span class="badge ${issue.severity}">${issue.severity.toUpperCase()}</span>`;

  if (issue.standard) {
    badges += `<span class="badge standard">${escapeHtml(issue.standard)}</span>`;
  }
  if (issue.riskLevel) {
    badges += `<span class="badge risk">RISK: ${issue.riskLevel.toUpperCase()}</span>`;
  }

  let meta = '';
  if (issue.location) {
    meta += `<div class="issue-meta"><strong>Location:</strong> ${escapeHtml(issue.location)}</div>`;
  }

  let recommendation = '';
  if (issue.recommendation) {
    recommendation = `<div class="recommendation"><strong>Recommendation:</strong> ${escapeHtml(issue.recommendation)}</div>`;
  }

  return `
    <div class="issue-item ${issue.severity}">
      <div class="issue-badges">${badges}</div>
      <div class="issue-description">${escapeHtml(issue.description)}</div>
      ${meta}
      ${recommendation}
    </div>
  `;
}

// Export HTML report
document.getElementById('exportBtn').addEventListener('click', () => {
  if (!window.currentHtmlReport) return;

  const blob = new Blob([window.currentHtmlReport], { type: 'text/html' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `qa-report-${Date.now()}.html`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
});

// Utility: Escape HTML
function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// Select All button functionality
document.querySelectorAll('.select-all-btn').forEach(btn => {
  btn.addEventListener('click', function() {
    const group = this.dataset.group;
    const checkboxGroup = document.querySelector(`[data-checkbox-group="${group}"]`);
    
    if (!checkboxGroup) return;
    
    const checkboxes = checkboxGroup.querySelectorAll('input[type="checkbox"]');
    const allChecked = Array.from(checkboxes).every(cb => cb.checked);
    
    // Toggle: if all are checked, uncheck all; otherwise check all
    checkboxes.forEach(cb => {
      cb.checked = !allChecked;
    });
    
    // Update button state
    this.classList.toggle('active', !allChecked);
    this.textContent = !allChecked ? 'Deselect All' : 'Select All';
  });

  // Initialize button state based on current checkbox states
  const group = btn.dataset.group;
  const checkboxGroup = document.querySelector(`[data-checkbox-group="${group}"]`);
  
  if (checkboxGroup) {
    const checkboxes = checkboxGroup.querySelectorAll('input[type="checkbox"]');
    
    // Update button state when any checkbox changes
    checkboxes.forEach(cb => {
      cb.addEventListener('change', () => {
        const allChecked = Array.from(checkboxes).every(c => c.checked);
        btn.classList.toggle('active', allChecked);
        btn.textContent = allChecked ? 'Deselect All' : 'Select All';
      });
    });

    // Set initial state
    const allChecked = Array.from(checkboxes).every(cb => cb.checked);
    btn.classList.toggle('active', allChecked);
    btn.textContent = allChecked ? 'Deselect All' : 'Select All';
  }
});

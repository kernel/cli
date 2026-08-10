package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const (
	auditLogExportStatusActive = "active"
	auditLogExportStatusPaused = "paused"
)

type auditLogExportListPageInfo struct {
	HasMore    bool
	NextOffset int
}

type AuditLogsExportService interface {
	Create(ctx context.Context, body kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error)
	List(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error)
	Get(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error)
	Update(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error)
	Delete(ctx context.Context, id string) error
	Test(ctx context.Context, id string) (*kernel.AuditLogExportDestinationTestResult, error)
}

type auditLogsExportClient struct {
	svc *kernel.AuditLogExportDestinationService
}

func (s *auditLogsExportClient) Create(ctx context.Context, body kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
	return s.svc.New(ctx, body)
}

func (s *auditLogsExportClient) List(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
	var httpRes *http.Response
	page, err := s.svc.List(ctx, query, option.WithResponseInto(&httpRes))
	if err != nil {
		return nil, auditLogExportListPageInfo{}, err
	}

	info, err := parseAuditLogExportListPagination(httpRes)
	if err != nil {
		return nil, auditLogExportListPageInfo{}, err
	}
	if page == nil {
		return nil, info, nil
	}
	return page.Items, info, nil
}

func parseAuditLogExportListPagination(response *http.Response) (auditLogExportListPageInfo, error) {
	if response == nil {
		return auditLogExportListPageInfo{}, fmt.Errorf("audit log export list response is missing pagination headers")
	}

	hasMoreValue := response.Header.Get("X-Has-More")
	hasMore, err := strconv.ParseBool(hasMoreValue)
	if err != nil {
		return auditLogExportListPageInfo{}, fmt.Errorf("invalid X-Has-More header %q", hasMoreValue)
	}

	nextOffsetValue := response.Header.Get("X-Next-Offset")
	nextOffset, err := strconv.Atoi(nextOffsetValue)
	if err != nil || nextOffset < 0 {
		return auditLogExportListPageInfo{}, fmt.Errorf("invalid X-Next-Offset header %q", nextOffsetValue)
	}
	if hasMore && nextOffset == 0 {
		return auditLogExportListPageInfo{}, fmt.Errorf("X-Has-More is true but X-Next-Offset is not positive")
	}
	if !hasMore && nextOffset != 0 {
		return auditLogExportListPageInfo{}, fmt.Errorf("X-Has-More is false but X-Next-Offset is %d", nextOffset)
	}

	return auditLogExportListPageInfo{HasMore: hasMore, NextOffset: nextOffset}, nil
}

func (s *auditLogsExportClient) Get(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error) {
	return s.svc.Get(ctx, id)
}

func (s *auditLogsExportClient) Update(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
	return s.svc.Update(ctx, id, body)
}

func (s *auditLogsExportClient) Delete(ctx context.Context, id string) error {
	return s.svc.Delete(ctx, id)
}

func (s *auditLogsExportClient) Test(ctx context.Context, id string) (*kernel.AuditLogExportDestinationTestResult, error) {
	return s.svc.Test(ctx, id)
}

type AuditLogsExportCmd struct {
	export AuditLogsExportService
}

type AuditLogsExportCreateInput struct {
	Region   string
	Bucket   string
	Prefix   string
	RoleARN  string
	KMSKeyID string
	Output   string
}

func (c AuditLogsExportCmd) Create(ctx context.Context, in AuditLogsExportCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	req := kernel.AuditLogExportDestinationNewParams{
		CreateAuditLogExportDestinationRequest: kernel.CreateAuditLogExportDestinationRequestParam{
			Type:    kernel.CreateAuditLogExportDestinationRequestTypeS3,
			Format:  kernel.CreateAuditLogExportDestinationRequestFormatJSONLGz,
			Region:  in.Region,
			Bucket:  in.Bucket,
			Prefix:  in.Prefix,
			RoleArn: in.RoleARN,
		},
	}
	if in.KMSKeyID != "" {
		req.CreateAuditLogExportDestinationRequest.KmsKeyID = kernel.String(in.KMSKeyID)
	}

	dest, err := c.export.Create(ctx, req)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(dest)
	}

	pterm.Success.Printf("Created audit log export destination %s (paused)\n", dest.ID)
	printAuditLogExportDestinationDetail(dest)
	pterm.Info.Printf("To activate this destination:\n  1. Update the trust policy of %s to allow %s as a principal, requiring sts:ExternalId = %s\n  2. Run: kernel audit-logs export test %s\n  3. Activate: kernel audit-logs export resume %s\n", dest.RoleArn, dest.KernelRoleArn, dest.ExternalID, dest.ID, dest.ID)
	return nil
}

type AuditLogsExportListInput struct {
	Limit  int
	Offset int
	Output string
}

func (c AuditLogsExportCmd) List(ctx context.Context, in AuditLogsExportListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Limit < 1 || in.Limit > 100 {
		return fmt.Errorf("--limit must be between 1 and 100")
	}
	if in.Offset < 0 {
		return fmt.Errorf("--offset must be non-negative")
	}

	query := kernel.AuditLogExportDestinationListParams{}
	if in.Limit > 0 {
		query.Limit = kernel.Opt(int64(in.Limit))
	}
	if in.Offset > 0 {
		query.Offset = kernel.Opt(int64(in.Offset))
	}
	destinations, pageInfo, err := c.export.List(ctx, query)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		data, err := marshalAuditLogExportListJSON(destinations, pageInfo.NextOffset)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(destinations) == 0 {
		pterm.Info.Println("No audit log export destinations found")
		return nil
	}

	table := pterm.TableData{{"ID", "Bucket", "Prefix", "Region", "Status", "Last Success", "Failures", "Last Error"}}
	for _, d := range destinations {
		table = append(table, []string{
			d.ID,
			d.Bucket,
			util.OrDash(d.Prefix),
			d.Region,
			string(d.Status),
			formatAuditLogExportTime(d.LastSuccessAt),
			strconv.FormatInt(d.ConsecutiveFailures, 10),
			truncateAuditLogExportError(d.LastError),
		})
	}
	PrintTableNoPad(table, true)

	if pageInfo.HasMore {
		pterm.Info.Printf("More destinations available; re-run with --offset %d\n", pageInfo.NextOffset)
	}
	return nil
}

func marshalAuditLogExportListJSON(destinations []kernel.AuditLogExportDestination, nextOffset int) ([]byte, error) {
	items := make([]json.RawMessage, 0, len(destinations))
	for _, destination := range destinations {
		raw := destination.RawJSON()
		if raw == "" {
			raw = "{}"
		}
		items = append(items, json.RawMessage(raw))
	}
	payload := struct {
		Destinations []json.RawMessage `json:"destinations"`
		NextOffset   int               `json:"next_offset,omitempty"`
	}{Destinations: items, NextOffset: nextOffset}
	return json.MarshalIndent(payload, "", "  ")
}

type AuditLogsExportGetInput struct {
	ID     string
	Output string
}

func (c AuditLogsExportCmd) Get(ctx context.Context, in AuditLogsExportGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	dest, err := c.export.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(dest)
	}

	printAuditLogExportDestinationDetail(dest)
	return nil
}

type AuditLogsExportUpdateInput struct {
	ID          string
	Region      *string
	Bucket      *string
	Prefix      *string
	RoleARN     *string
	KMSKeyID    *string
	ClearKMSKey bool
	Output      string
}

func (c AuditLogsExportCmd) Update(ctx context.Context, in AuditLogsExportUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.KMSKeyID != nil && in.ClearKMSKey {
		return fmt.Errorf("cannot specify both --kms-key-id and --clear-kms-key")
	}

	req := kernel.UpdateAuditLogExportDestinationRequestParam{}
	if in.Region != nil {
		req.Region = kernel.String(*in.Region)
	}
	if in.Bucket != nil {
		req.Bucket = kernel.String(*in.Bucket)
	}
	if in.Prefix != nil {
		req.Prefix = kernel.String(*in.Prefix)
	}
	if in.RoleARN != nil {
		req.RoleArn = kernel.String(*in.RoleARN)
	}
	if in.KMSKeyID != nil {
		req.KmsKeyID = kernel.String(*in.KMSKeyID)
	}
	if in.ClearKMSKey {
		req.KmsKeyID = kernel.String("")
	}
	if !req.Region.Valid() && !req.Bucket.Valid() && !req.Prefix.Valid() && !req.RoleArn.Valid() && !req.KmsKeyID.Valid() {
		return fmt.Errorf("nothing to update: pass at least one of --region, --bucket, --prefix, --role-arn, --kms-key-id, or --clear-kms-key")
	}

	dest, err := c.export.Update(ctx, in.ID, kernel.AuditLogExportDestinationUpdateParams{UpdateAuditLogExportDestinationRequest: req})
	if err != nil {
		return cleanedUpAuditLogExportUpdateError(err)
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(dest)
	}

	pterm.Success.Printf("Updated audit log export destination %s\n", dest.ID)
	printAuditLogExportDestinationDetail(dest)
	return nil
}

type AuditLogsExportStatusInput struct {
	ID     string
	Status string
	Output string
}

func (c AuditLogsExportCmd) SetStatus(ctx context.Context, in AuditLogsExportStatusInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Status != auditLogExportStatusActive && in.Status != auditLogExportStatusPaused {
		return fmt.Errorf("invalid status %q", in.Status)
	}

	dest, err := c.export.Update(ctx, in.ID, kernel.AuditLogExportDestinationUpdateParams{
		UpdateAuditLogExportDestinationRequest: kernel.UpdateAuditLogExportDestinationRequestParam{
			Status: kernel.UpdateAuditLogExportDestinationRequestStatus(in.Status),
		},
	})
	if err != nil {
		return cleanedUpAuditLogExportUpdateError(err)
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(dest)
	}

	if in.Status == auditLogExportStatusActive {
		pterm.Success.Printf("Resumed audit log export destination %s\n", dest.ID)
	} else {
		pterm.Success.Printf("Paused audit log export destination %s\n", dest.ID)
	}
	printAuditLogExportDestinationDetail(dest)
	if in.Status == auditLogExportStatusPaused {
		pterm.Info.Println("An S3 upload already in progress may still complete; its rows can appear again after the destination is resumed.")
	}
	return nil
}

type AuditLogsExportDeleteInput struct {
	ID string
}

func (c AuditLogsExportCmd) Delete(ctx context.Context, in AuditLogsExportDeleteInput) error {
	if err := c.export.Delete(ctx, in.ID); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted audit log export destination %s\n", in.ID)
	return nil
}

type AuditLogsExportTestInput struct {
	ID     string
	Output string
}

func (c AuditLogsExportCmd) Test(ctx context.Context, in AuditLogsExportTestInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	res, err := c.export.Test(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if err := util.PrintPrettyJSON(res); err != nil {
			return err
		}
	} else if res.Success {
		pterm.Success.Printf("Test passed (stage: %s)\n", res.Stage)
	} else if res.Error.Code != "" || res.Error.Message != "" {
		pterm.Error.Printf("Test failed at stage %s: %s: %s\n", res.Stage, res.Error.Code, res.Error.Message)
	} else {
		pterm.Error.Printf("Test failed at stage %s\n", res.Stage)
	}

	if !res.Success {
		return fmt.Errorf("audit log export destination test failed at stage %s", res.Stage)
	}
	return nil
}

func cleanedUpAuditLogExportUpdateError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		return fmt.Errorf("%w (destination changed concurrently; re-run against fresh state)", util.CleanedUpSdkError{Err: err})
	}
	return util.CleanedUpSdkError{Err: err}
}

func printAuditLogExportDestinationDetail(d *kernel.AuditLogExportDestination) {
	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", d.ID},
		{"Type", string(d.Type)},
		{"Region", d.Region},
		{"Bucket", d.Bucket},
		{"Prefix", util.OrDash(d.Prefix)},
		{"Role ARN", d.RoleArn},
		{"Kernel Role ARN", d.KernelRoleArn},
		{"External ID", d.ExternalID},
		{"KMS Key ID", util.OrDash(d.KmsKeyID)},
		{"Format", string(d.Format)},
		{"Status", string(d.Status)},
		{"Last Exported Cursor", util.OrDash(d.LastExportedCursor)},
		{"Last Success", formatAuditLogExportLastSuccess(d.LastSuccessAt)},
		{"Last Error", util.OrDash(d.LastError)},
		{"Last Error At", formatAuditLogExportTime(d.LastErrorAt)},
		{"Consecutive Failures", strconv.FormatInt(d.ConsecutiveFailures, 10)},
		{"Next Attempt", formatAuditLogExportTime(d.NextAttemptAt)},
		{"Created At", util.FormatLocal(d.CreatedAt)},
		{"Updated At", util.FormatLocal(d.UpdatedAt)},
	}
	PrintTableNoPad(rows, true)
}

func formatAuditLogExportTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return util.FormatLocal(t)
}

func formatAuditLogExportLastSuccess(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	lag := max(time.Since(t).Round(time.Second), 0)
	return fmt.Sprintf("%s (%s ago)", util.FormatLocal(t), lag)
}

func truncateAuditLogExportError(s string) string {
	const maxLen = 60
	runes := []rune(s)
	if len(runes) <= maxLen {
		return util.OrDash(s)
	}
	return string(runes[:maxLen-3]) + "..."
}

func getAuditLogsExportHandler(cmd *cobra.Command) AuditLogsExportCmd {
	client := getKernelClient(cmd)
	return AuditLogsExportCmd{export: &auditLogsExportClient{svc: &client.AuditLogs.ExportDestinations}}
}

func runAuditLogsExportCreate(cmd *cobra.Command, args []string) error {
	region, _ := cmd.Flags().GetString("region")
	bucket, _ := cmd.Flags().GetString("bucket")
	prefix, _ := cmd.Flags().GetString("prefix")
	roleARN, _ := cmd.Flags().GetString("role-arn")
	kmsKeyID, _ := cmd.Flags().GetString("kms-key-id")
	output, _ := cmd.Flags().GetString("output")
	c := getAuditLogsExportHandler(cmd)
	return c.Create(cmd.Context(), AuditLogsExportCreateInput{
		Region:   region,
		Bucket:   bucket,
		Prefix:   prefix,
		RoleARN:  roleARN,
		KMSKeyID: kmsKeyID,
		Output:   output,
	})
}

func runAuditLogsExportList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	output, _ := cmd.Flags().GetString("output")
	c := getAuditLogsExportHandler(cmd)
	return c.List(cmd.Context(), AuditLogsExportListInput{Limit: limit, Offset: offset, Output: output})
}

func runAuditLogsExportGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	c := getAuditLogsExportHandler(cmd)
	return c.Get(cmd.Context(), AuditLogsExportGetInput{ID: args[0], Output: output})
}

func runAuditLogsExportUpdate(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	clearKMSKey, _ := cmd.Flags().GetBool("clear-kms-key")
	in := AuditLogsExportUpdateInput{ID: args[0], ClearKMSKey: clearKMSKey, Output: output}
	if cmd.Flags().Changed("region") {
		region, _ := cmd.Flags().GetString("region")
		in.Region = &region
	}
	if cmd.Flags().Changed("bucket") {
		bucket, _ := cmd.Flags().GetString("bucket")
		in.Bucket = &bucket
	}
	if cmd.Flags().Changed("prefix") {
		prefix, _ := cmd.Flags().GetString("prefix")
		in.Prefix = &prefix
	}
	if cmd.Flags().Changed("role-arn") {
		roleARN, _ := cmd.Flags().GetString("role-arn")
		in.RoleARN = &roleARN
	}
	if cmd.Flags().Changed("kms-key-id") {
		kmsKeyID, _ := cmd.Flags().GetString("kms-key-id")
		in.KMSKeyID = &kmsKeyID
	}
	c := getAuditLogsExportHandler(cmd)
	return c.Update(cmd.Context(), in)
}

func runAuditLogsExportPause(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	c := getAuditLogsExportHandler(cmd)
	return c.SetStatus(cmd.Context(), AuditLogsExportStatusInput{ID: args[0], Status: auditLogExportStatusPaused, Output: output})
}

func runAuditLogsExportResume(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	c := getAuditLogsExportHandler(cmd)
	return c.SetStatus(cmd.Context(), AuditLogsExportStatusInput{ID: args[0], Status: auditLogExportStatusActive, Output: output})
}

func runAuditLogsExportDelete(cmd *cobra.Command, args []string) error {
	c := getAuditLogsExportHandler(cmd)
	return c.Delete(cmd.Context(), AuditLogsExportDeleteInput{ID: args[0]})
}

func runAuditLogsExportTest(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	c := getAuditLogsExportHandler(cmd)
	return c.Test(cmd.Context(), AuditLogsExportTestInput{ID: args[0], Output: output})
}

var auditLogsExportCmd = &cobra.Command{
	Use:     "export",
	Aliases: []string{"exports", "export-destinations"},
	Short:   "Manage audit log export destinations",
	Long: "Manage S3 destinations that receive a continuous export of your organization's audit logs.\n\n" +
		"Objects are written as <prefix>/destination_id=<destination>/org_id=<org>/date=<YYYY-MM-DD>/hour=<HH>/<window>-<chunk>.jsonl.gz. " +
		"Delivery is at-least-once; consumers must deduplicate on event_id.",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var auditLogsExportCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an S3 audit log export destination",
	Long:  "Create an S3 audit log export destination. The destination is created paused; test it and then activate it with 'kernel audit-logs export resume <id>'.",
	Args:  cobra.NoArgs,
	RunE:  runAuditLogsExportCreate,
}

var auditLogsExportListCmd = &cobra.Command{
	Use:   "list",
	Short: "List audit log export destinations",
	Args:  cobra.NoArgs,
	RunE:  runAuditLogsExportList,
}

var auditLogsExportGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get details of an audit log export destination",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuditLogsExportGet,
}

var auditLogsExportUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update an audit log export destination",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuditLogsExportUpdate,
}

var auditLogsExportPauseCmd = &cobra.Command{
	Use:   "pause <id>",
	Short: "Pause an audit log export destination",
	Long:  "Pause an audit log export destination. Pausing prevents new delivery attempts; an S3 upload already in progress may still complete.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuditLogsExportPause,
}

var auditLogsExportResumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Resume an audit log export destination",
	Long:  "Resume an audit log export destination. Delivery starts from the time of the resume; events recorded while paused are not exported.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuditLogsExportResume,
}

var auditLogsExportDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete an audit log export destination",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuditLogsExportDelete,
}

var auditLogsExportTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Test an audit log export destination",
	Long:  "Test an audit log export destination by assuming its role and writing a test object. Exits non-zero when the test fails.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuditLogsExportTest,
}

func init() {
	addJSONOutputFlag(auditLogsExportCreateCmd)
	auditLogsExportCreateCmd.Flags().String("region", "", "AWS region of the destination bucket (required)")
	_ = auditLogsExportCreateCmd.MarkFlagRequired("region")
	auditLogsExportCreateCmd.Flags().String("bucket", "", "Destination S3 bucket name (required)")
	_ = auditLogsExportCreateCmd.MarkFlagRequired("bucket")
	auditLogsExportCreateCmd.Flags().String("prefix", "", "Key prefix for exported objects; may be empty (required)")
	_ = auditLogsExportCreateCmd.MarkFlagRequired("prefix")
	auditLogsExportCreateCmd.Flags().String("role-arn", "", "IAM role ARN Kernel assumes to deliver logs (required)")
	_ = auditLogsExportCreateCmd.MarkFlagRequired("role-arn")
	auditLogsExportCreateCmd.Flags().String("kms-key-id", "", "KMS key ID, alias, or ARN for server-side encryption")

	addJSONOutputFlag(auditLogsExportListCmd)
	auditLogsExportListCmd.Flags().Int("limit", 20, "Maximum number of destinations to return (1-100)")
	auditLogsExportListCmd.Flags().Int("offset", 0, "Number of destinations to skip (for pagination)")

	addJSONOutputFlag(auditLogsExportGetCmd)

	addJSONOutputFlag(auditLogsExportUpdateCmd)
	auditLogsExportUpdateCmd.Flags().String("region", "", "Update the AWS region of the destination bucket")
	auditLogsExportUpdateCmd.Flags().String("bucket", "", "Update the destination S3 bucket name")
	auditLogsExportUpdateCmd.Flags().String("prefix", "", "Update the key prefix for exported objects")
	auditLogsExportUpdateCmd.Flags().String("role-arn", "", "Update the IAM role ARN Kernel assumes to deliver logs")
	auditLogsExportUpdateCmd.Flags().String("kms-key-id", "", "Update the KMS key ID, alias, or ARN for server-side encryption")
	auditLogsExportUpdateCmd.Flags().Bool("clear-kms-key", false, "Remove the configured KMS key")
	auditLogsExportUpdateCmd.MarkFlagsMutuallyExclusive("kms-key-id", "clear-kms-key")

	addJSONOutputFlag(auditLogsExportPauseCmd)
	addJSONOutputFlag(auditLogsExportResumeCmd)
	addJSONOutputFlag(auditLogsExportTestCmd)

	auditLogsExportCmd.AddCommand(auditLogsExportCreateCmd)
	auditLogsExportCmd.AddCommand(auditLogsExportListCmd)
	auditLogsExportCmd.AddCommand(auditLogsExportGetCmd)
	auditLogsExportCmd.AddCommand(auditLogsExportUpdateCmd)
	auditLogsExportCmd.AddCommand(auditLogsExportPauseCmd)
	auditLogsExportCmd.AddCommand(auditLogsExportResumeCmd)
	auditLogsExportCmd.AddCommand(auditLogsExportDeleteCmd)
	auditLogsExportCmd.AddCommand(auditLogsExportTestCmd)

	auditLogsCmd.AddCommand(auditLogsExportCmd)
}

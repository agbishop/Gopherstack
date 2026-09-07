service: medialive
sdk_module: aws-sdk-go-v2/service/medialive@v1.101.4   # version audited against
last_audit_commit: 6c48ab50cb35a7b8834b7fea50407931c6df3119  # gopherstack-7ux2 (2026-08-13) fixed after this hash was recorded; hash not yet known at edit time
last_audit_date: 2026-08-23
overall: A            # Sweep 6 (gopherstack-jb9i): Channel now models all 17
                       # CreateChannelInput/UpdateChannelInput top-level members (was 5) --
                       # CdiInputSpecification/ChannelEngineVersion/ChannelSecurityGroups/
                       # Destinations/EncoderSettings/InferenceSettings/InputAttachments/
                       # InputSpecification/LinkedChannelSettings/LogLevel/Maintenance/Vpc added,
                       # each field-diffed against the SDK's serializers.go/deserializers.go/
                       # types.go and round-tripped through a real aws-sdk-go-v2 client (see
                       # Channel's note below and handler_channels_test.go's
                       # TestChannel_ExtendedFieldsSDKRoundTrip). Also added the
                       # anywhereSettings.channelPlacementGroupId existence validation sweep 5
                       # flagged as a residual gap (400 BadRequestException on an unknown group,
                       # matching the existing clusterId validation). EncoderSettings itself is
                       # modeled to a deliberately bounded depth -- see Channel's note and the
                       # "gaps" list below for exactly what is and is not modeled, and why.
                       # Previous sweep 5 summary follows, unchanged:
                       # Parity sweep 5: closed every concrete gap sweep 4 left open (InputDevice
                       # request-body casing, Cluster.ChannelIds/Node.ChannelPlacementGroups/
                       # ChannelPlacementGroup.Channels derivation, CW/EB TemplateGroup
                       # templateCount, EventBridgeRuleTemplate eventTargetCount, Reservation
                       # renewalSettings) and independently field-diffed two families the prior
                       # sweep did NOT audit deeply enough to catch: Cluster was missing
                       # "networkSettings" entirely (a real CreateClusterInput/
                       # DescribeClusterOutput field), and Channel was missing
                       # "anywhereSettings" entirely (the field needed to derive the
                       # channelIds/channels/channelPlacementGroups associations above from
                       # something real instead of a hardcoded empty list). Also found and
                       # fixed two leaks: b.tags[ARN] rows were never removed on delete for
                       # every resource family outside the Channel/Input/InputSecurityGroup/
                       # Multiplex/InputDevice fast path (Cluster/Node/SignalMap/
                       # CloudWatchAlarmTemplate(Group)/EventBridgeRuleTemplate(Group)/
                       # Reservation/Network/SdiSource/ChannelPlacementGroup), and
                       # DeleteCluster never cascade-deleted its ChannelPlacementGroups (a
                       # separate top-level table, unlike Nodes which are embedded in
                       # storedCluster and vanish automatically). Prior sweep 4's wire-shape
                       # casing bug (PascalCase-vs-lowerCamel)
                       # that was fixed for Channel/Multiplex/MultiplexProgram/Tags in the
                       # prior pass is now fixed for EVERY remaining family in this service
                       # -- Cluster, Node, ChannelPlacementGroup, SignalMap,
                       # CloudWatchAlarmTemplate(Group), EventBridgeRuleTemplate(Group),
                       # Offering, Reservation, Network, SdiSource, Batch*, Schedule,
                       # Alerts/AccountConfiguration/Versions, and InputDevice's own
                       # output-struct casing. Also fixed: BatchStart/BatchStop parsing a
                       # nonexistent InputIds field (dead code) while BatchDelete was
                       # missing InputSecurityGroupIds parsing (real gap), and
                       # SignalMap/CloudWatchAlarmTemplate(Group)/EventBridgeRuleTemplate
                       # (Group) missing createdAt/modifiedAt entirely. medialive's
                       # wire-shape casing gap (this service's single largest outstanding
                       # parity issue as of the prior pass) is now closed.

# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
families:
  RouteMatcher:
    status: ok
    note: >
      Exhaustively diffed all 123 routed ops' (method, path) pairs against
      aws-sdk-go-v2/service/medialive@v1.101.4's serializers.go (every
      awsRestjson1_serializeOp*.HandleSerialize's httpbinding.SplitURI call +
      request.Method). classifyPath's op set matches the SDK's op set exactly
      (123/123), and every path-template + HTTP-method pair matches exactly,
      including tricky List-vs-Create collisions on shared collection paths
      (e.g. GET/POST /prod/channels, /prod/multiplexes, /prod/clusters) and
      sub-action paths (start/stop/accept/cancel/reboot/reject/transfer under
      /prod/inputDevices/{id}/..., channelClass/restartChannelPipelines/
      schedule under /prod/channels/{id}/..., monitor-deployment under
      /prod/signal-maps/{id}/...). No route-matcher bugs found in this
      service -- the class of bug that hit backup/eks/s3control/guardduty/
      cleanrooms/bedrockagent/iotwireless does not reproduce here.
      gopherstack-jqh2: this manual diff is now a permanent regression test,
      TestExtractOperation_SDKRouteTable (handler_paths_sdk_diff_test.go),
      table-driven over all 123 real ops -- re-run and reconfirmed 123/123
      clean.
  Channel:
    status: ok
    note: >
      FIXED in a prior pass. CreateChannel/DescribeChannel/UpdateChannel/
      DeleteChannel/ListChannels/StartChannel/StopChannel/UpdateChannelClass/
      RestartChannelPipelines all had PascalCase JSON keys ("Arn"/"Id"/"Name"/
      "ChannelClass"/"RoleArn"/"State"/"Tags", wrapper key "Channel") that
      the real aws-sdk-go-v2 client's restjson1 deserializer -- which
      switches on an exact-case string per generated field ("arn"/"id"/
      "name"/... , wrapper key "channel") -- silently ignores. Fixed by
      lowercasing channelOutput's json tags and the handleListChannels
      summary map + flipping keyChannel to "channel". This pass (sweep 4)
      merged the shared keyArn/keyID/keyName/keyState/keyTags constants
      (previously PascalCase, kept separate from Channel's own wireArn/
      wireID/wireName/wireState) into a single lowerCamel set now that
      every family in the service uses the same casing -- see "Notes"
      below.
      SWEEP 5: independently field-diffed CreateChannelInput/
      UpdateChannelInput/ChannelSummary against the SDK and found Channel
      had NO "anywhereSettings" field at all (types.AnywhereSettings /
      types.DescribeAnywhereSettings: clusterId/channelPlacementGroupId) --
      CreateChannel/UpdateChannel silently dropped a caller's
      anywhereSettings, and this was the root cause blocking
      Cluster.channelIds/ChannelPlacementGroup.channels/Node.
      channelPlacementGroups from ever being anything but a hardcoded empty
      list (see those families below). Added Channel.AnywhereSettings
      (ClusterID/ChannelPlacementGroupID), wired into Create/UpdateChannel
      request parsing (with ClusterID existence validation -> 400 on an
      unknown cluster, matching AWS's reference-validation behavior) and
      Describe/Create/Update/List output ("anywhereSettings" omitted
      entirely when unset, matching a non-Anywhere Channel).
      SWEEP 6 (gopherstack-jb9i): closed the remaining 12-of-17
      CreateChannelInput/UpdateChannelInput member gap sweep 5 left OUT OF
      SCOPE. Every member below was field-diffed against
      aws-sdk-go-v2/service/medialive@v1.101.4's serializers.go/
      deserializers.go/types.go and is now wired into Create/UpdateChannel
      request parsing and Describe/Create/Update/Delete/Start/Stop/List
      output (each nested object omitted entirely when unset, same
      "has-the-key-at-all" convention as anywhereSettings above; each Update
      field paired with its own HasX flag so an omitted key preserves the
      existing value -- see ChannelUpdateExtras in interfaces.go):
        - CdiInputSpecification, ChannelEngineVersion, ChannelSecurityGroups,
          InputSpecification, LogLevel, Maintenance: flat/near-flat shapes,
          modeled in full. Vpc: request fields (subnetIds/
          publicAddressAllocationIds/securityGroupIds) modeled; the response
          shape's availabilityZones/networkInterfaceIds (computed by a real
          VPC/ENI integration gopherstack doesn't have) are always omitted,
          never fabricated -- same convention as ChannelEngineVersion's
          expirationDate.
        - InferenceSettings, LinkedChannelSettings: modeled in full,
          including LinkedChannelSettings.PrimaryChannelSettings.
          followingChannelArns, which -- like Cluster.ChannelIds/
          ChannelPlacementGroup.Channels -- is derived live at read time
          (channels.go's followingChannelArns, scanning every other
          channel's Follower.PrimaryChannelArn) rather than stored, since
          that's how the real API computes it too.
        - Destinations (types.OutputDestination): modeled in full, including
          every one of its 5 per-technology settings variants (standard/
          MediaPackage/Multiplex/MediaConnectRouter/SRT) -- none of them are
          deep unions themselves, so there was no "genuinely impractical"
          carve-out needed here.
        - InputAttachments (types.InputAttachment): InputAttachmentName/
          InputId/LogicalInterfaceNames and the full
          AutomaticInputFailoverSettings tree (including all 3 named
          failover-condition variants: AudioSilenceSettings/
          InputLossSettings/VideoBlackSettings) are modeled.
          InputSettings (gopherstack-sthr, this pass) is now ALSO modeled
          in full: AudioSelectors (the 4-variant AudioSelectorSettings
          union -- AudioHlsRenditionSelection/AudioLanguageSelection/
          AudioPidSelection/AudioTrackSelection -- including
          AudioPreMixerSettings/RemixSettings/AudioNormalizationSettings
          nesting under AudioPidSelection.Pids/AudioTrackSelection.Tracks),
          CaptionSelectors (the 8-variant CaptionSelectorSettings union --
          Ancillary/Arib/DvbSub/Embedded/Scte20/Scte27/SmartSubtitle/
          Teletext, Arib as a bool "variant is set" marker since it has no
          wire fields), VideoSelector (ColorSpace/ColorSpaceUsage plus the
          1-variant ColorSpaceSettings union -- Hdr10Settings -- and the
          2-variant SelectorSettings union -- VideoSelectorPid/
          VideoSelectorProgramId), NetworkInputSettings (HlsInputSettings/
          MulticastInputSettings/ServerValidation), and the flat filter
          fields (DeblockFilter/DenoiseFilter/FilterStrength/InputFilter/
          Scte35Pid/Smpte2038DataPreference/SourceEndBehavior). This turned
          out much smaller than the bd issue's original "comparable in
          depth to EncoderSettings' codec settings" framing suggested --
          every sub-shape is flat scalars or a small tagged union once read
          from the pinned SDK source, verified against v1.101.4. See
          services/medialive/handler_channels_input_settings.go and
          interfaces.go's InputSettings doc comment.
        - EncoderSettings: modeled to a deliberately bounded depth. Fully
          modeled: TimecodeConfig, AvailBlanking, BlackoutSlate,
          FeatureActivations, GlobalConfiguration (incl. InputLossBehavior
          and the 3-variant OutputLockingSettings union), ThumbnailConfiguration,
          AvailConfiguration (AvailSettings' 3-variant union -- Esam/
          Scte35SpliceInsert/Scte35TimeSignalApos -- plus
          Scte35SegmentationScope), ColorCorrectionSettings
          (GlobalColorCorrections' InputColorSpace/OutputColorSpace/Uri),
          MotionGraphicsConfiguration (MotionGraphicsInsertion plus the
          MotionGraphicsSettings union, which the real SDK currently defines
          exactly one variant of -- HtmlMotionGraphicsSettings, itself an
          empty marker object), NielsenConfiguration (DistributorId/
          NielsenPcmToId3Tagging), and the flat/enum fields of
          VideoDescriptions/CaptionDescriptions/OutputGroups (OutputGroup.
          Name + each Output's OutputName/VideoDescriptionName/
          AudioDescriptionNames/CaptionDescriptionNames -- purely
          referential fields, not settings blobs).
          AudioDescriptions (gopherstack-sthr, this pass) is now modeled in
          full: the flat/enum fields (as before) PLUS CodecSettings (the
          AudioCodecSettings union -- AacSettings/Ac3Settings/
          Eac3AtmosSettings/Eac3Settings/Mp2Settings/WavSettings/
          PassThroughSettings, the last as an empty-marker bool like Arib
          above), AudioNormalizationSettings and RemixSettings (the SAME
          domain types InputSettings' AudioPreMixerSettings tree uses --
          types.AudioNormalizationSettings/types.RemixSettings are the
          identical SDK shapes in both places, so no new types were needed),
          AudioWatermarkingSettings (NielsenWatermarksSettings' NielsenCBET/
          NielsenNaesIiNw pair), AudioDashRoles, and DvbDashAccessibility.
          The bd issue's title called this a "~20-variant AudioCodecSettings
          union" -- verified against the pinned SDK it is actually 7
          variants, all flat scalar structs (the largest, Eac3Settings, is
          20 scalar fields with no further nesting), so it was tractable in
          this pass unlike the three unions still excluded below.
          OutputGroup.OutputGroupSettings and Output.OutputSettings
          (gopherstack-hj9n, this pass) are now BOTH modeled in full,
          together -- deferred from gopherstack-sthr specifically because
          modeling one half of this correlated pair (a group's delivery
          settings and its outputs' delivery settings describe the same
          output technology) would misrepresent what a channel round-trips.
          types.OutputGroupSettings' 11 variants (Archive/CmafIngest/
          FrameCapture/Hls/MediaConnectRouter/MediaPackage/MsSmooth/
          Multiplex/Rtmp/Srt/Udp) and types.OutputSettings' 11 matching
          variants are modeled down through every nested container/CDN/
          stream sub-union each variant references (M2tsSettings,
          MultiplexM2tsSettings, HlsSettings, HlsCdnSettings,
          KeyProviderSettings, ArchiveCdnSettings, FrameCaptureCdnSettings,
          M3u8Settings, MediaPackageV2GroupSettings/
          MediaPackageV2DestinationSettings) -- see
          handler_channels_outputs.go and interfaces.go's per-type doc
          comments for the full field/file:line inventory. Two
          empty-marker variants (types.RawSettings under
          ArchiveContainerSettings, types.MultiplexGroupSettings itself)
          use the same *emptyMarker-pointer convention as
          PassThroughSettings above, verified round-tripping under
          omitempty by TestChannel_OutputSettings.
          CaptionDescription.DestinationSettings (gopherstack-1szb, first
          sub-pass) is now modeled in full: types.CaptionDestinationSettings
          (types.go:1151) is actually 13 variants, not the 12 the bd issue
          counted -- 8 are empty-marker structs (Arib/Embedded/
          EmbeddedPlusScte20/RtmpCaptionInfo/Scte20PlusEmbedded/Scte27/
          SmpteTt/Teletext, same *emptyMarker-pointer convention as
          PassThroughSettings above), Ttml and Webvtt are single-field
          (styleControl), EbuTtD is 6 fields, and BurnIn/DvbSub (types.go:998,
          :2216) are 18 fields each (not 19 as originally estimated) sharing
          one font/color/outline/shadow/position field set under separate
          DVB-Sub-scoped enums. CaptionDescription's CaptionDashRoles is
          modeled alongside it. Proof: handler_channels_captions_test.go's
          TestChannel_CaptionDestinationSettings drives a real
          aws-sdk-go-v2 client through every variant family.
          VideoDescription.CodecSettings (types.VideoCodecSettings,
          types.go:8582, gopherstack-1szb final sub-pass) is the last
          EncoderSettings union and is now modeled in full: 5 variants,
          measured field-by-field against the serializer rather than the bd
          issue's estimate -- Av1Settings 24 fields (not 25), H264Settings
          44 (not 45), H265Settings 42 (not 43), Mpeg2Settings 17 (not 18),
          FrameCaptureSettings 3 (not 4). All five share TimecodeBurninSettings
          (types.go:8411, 3 fields). Av1/H264/H265 each carry their own
          ColorSpaceSettings sub-union (5/3/6 variants -- NOT shared as one
          Go type since the variant sets genuinely differ: H264 lacks
          Hdr10/Hlg2020/DolbyVision81, Av1 lacks DolbyVision81); Hdr10Settings
          is reused from the pre-existing VideoSelectorColorSpaceSettings
          modeling (types.Hdr10Settings is the identical SDK shape in both
          places). H264's and H265's FilterSettings sub-union IS identical
          between the two (BandwidthReductionFilterSettings +
          TemporalFilterSettings) and shares one wire struct/extractor pair,
          same convention as burnInAndDvbSubOutput. Mpeg2 has no
          ColorSpaceSettings union (ColorSpace is a plain enum there); its
          FilterSettings wraps only TemporalFilterSettings. Nothing was
          silently dropping this data before -- CodecSettings was cleanly
          absent (no struct field existed to hold it), verified by a
          pre-fix run of the new test against a git-worktree checkout of the
          prior commit (every subtest failed on a nil CodecSettings, not a
          wrong-shape assertion). Proof:
          handler_channels_video_codec_test.go's TestChannel_VideoCodecSettings
          drives a real aws-sdk-go-v2 client through all 5 variants,
          including a shared-Hdr10Settings/BandwidthReductionFilterSettings
          case (H265) and empty-marker color-space variants (Av1's
          Hlg2020Settings, H264's Rec709Settings).
      Also closed the "anywhereSettings.channelPlacementGroupId is accepted
      without existence validation" gap sweep 5 flagged: Create/UpdateChannel
      now validate it against real ChannelPlacementGroup state the same way
      clusterId was already validated (400 BadRequestException on an unknown
      group, via the new validateAnywhereSettings helper in channels.go).
      Proof: handler_channels_test.go's TestChannel_ExtendedFieldsSDKRoundTrip
      drives a real aws-sdk-go-v2 client through the actual router path for
      every family above (flat fields, inferenceSettings, destinations,
      inputAttachments incl. all 3 failover variants, encoderSettings'
      modeled subset, linkedChannelSettings' derived followingChannelArns,
      and Update's has-flag preservation semantics) --
      TestAnywhereSettings_ChannelPlacementGroupValidation covers the new
      validation.
  Input:
    status: ok
    note: >
      Verified only, no changes needed. CreateInput/DescribeInput/
      UpdateInput/DeleteInput/ListInputs already used the correct lowerCamel
      wire keys and "input" wrapper.
  InputSecurityGroup:
    status: ok
    note: Verified only, no changes needed. Already correct lowerCamel wire keys ("arn"/"id"/"state"/"whitelistRules"/"cidr"), "securityGroup" wrapper.
  Multiplex:
    status: ok
    note: >
      FIXED in a prior pass, same bug class as Channel. multiplexOutput's
      tags and the nested MultiplexSettings were all PascalCase; fixed to
      lowerCamel, including the CreateMultiplex/UpdateMultiplex REQUEST body
      parsing. Also added the wire's missing `pipelinesRunningCount` and
      `programCount`.
  MultiplexProgram:
    status: ok
    note: >
      FIXED in a prior pass, same bug class. multiplexProgramOutput and its
      nested settings were PascalCase; fixed to lowerCamel, plus the
      corresponding request-body parsing and the "MultiplexProgram" wrapper
      key (now "multiplexProgram").
  Tags:
    status: partial
    note: >
      Two independent bugs from the prior pass, one fixed fully and one
      fixed for its highest-traffic call site at the time (now fully
      resolved, see below). (1) STATE BUG (fixed): CreateTags/DeleteTags/
      ListTagsForResource operated on a b.tags[ARN] map disconnected from
      the per-resource `.Tags` field. Fixed via `taggableResourceTags(arn)`
      (backend.go) + `findLiveTags` helper. (2) WIRE BUG (now fully fixed):
      ListTagsForResourceOutput's real key is lowercase "tags"; the prior
      pass fixed only the ListTagsForResource handler's literal (leaving
      the shared `keyTags` constant as "Tags" for other families). This
      pass (sweep 4) flipped `keyTags` itself to "tags" now that every
      family sharing it has been fixed to the same casing -- see Notes.
  InputDevice:
    status: ok
    note: >
      Tag-store sync bug (see Tags above) was fixed in a prior pass and NOT
      touched this pass. Wire-shape casing (inputDeviceOutput's field tags,
      the ListInputDevices "InputDevices" wrapper, the
      ListInputDeviceTransfers "InputDeviceTransfers" wrapper and its item
      keys, and "MaintenanceWindowActive") is FIXED this pass -- all
      lowerCamel now ("tags"/"arn"/"id"/"name"/"serialNumber"/"macAddress"/
      "type"/"connectionState"/"deviceSettingsSyncState"/
      "deviceUpdateStatus"/"maintenanceWindowActive", wrappers
      "inputDevices"/"inputDeviceTransfers"). Deliberately NOT touched this
      pass (out of scope per this sweep's task boundary): request-body
      parsing for ClaimDevice/UpdateInputDevice/TransferInputDevice (still
      reads PascalCase "Id"/"TargetCustomerId"/"TargetRegion"/
      "TransferMessage" -- verified these ARE the wrong casing vs the real
      TransferInputDeviceInput serializer, which uses "targetCustomerId"/
      "targetRegion"/"transferMessage"; tracked as a residual gap below).
      Route/method matching for all InputDevice sub-actions verified
      correct in the prior pass.
      SWEEP 5: FIXED the residual request-body casing gap noted above.
      handleClaimDevice now reads "id" (was "Id" -- verified against
      awsRestjson1_serializeOpDocumentClaimDeviceInput, which sends only
      "id"); ClaimDevice silently no-oped on every real caller before this
      fix. handleTransferInputDevice now reads "targetCustomerId"/
      "targetRegion"/"transferMessage" (was "TargetCustomerId"/
      "TargetRegion"/"TransferMessage" -- verified against
      awsRestjson1_serializeOpDocumentTransferInputDeviceInput). Re-checked
      UpdateInputDevice's request body: it already read "name" correctly
      (the PARITY.md gap note bundling it in with ClaimDevice/
      TransferInputDevice was imprecise -- UpdateInputDeviceInput's other
      real fields, availabilityZone/hdDeviceSettings/uhdDeviceSettings,
      remain unhandled, consistent with this service's existing
      minimal-model approach for deeply nested device-settings objects).
      gopherstack-7ux2: REMOVED "maintenanceWindowActive" outright.
      SWEEP 4 had only fixed its casing while leaving it in place (see the
      prior handler comment, now deleted); this pass confirmed against
      both deserializer field switches (medialive@v1.101.4
      types/types.go:4498-4556 InputDeviceSummary,
      api_op_DescribeInputDevice.go DescribeInputDeviceOutput) that neither
      shape has ever had this member -- ListInputDevices and
      DescribeInputDevice were both emitting a field no real client
      expects. It had no reader anywhere in this package either (only ever
      set true by StartInputDeviceMaintenanceWindow and never checked), so
      removal touched the domain model, the persisted-device shape, and the
      wire struct with nothing left dangling; StartInputDeviceMaintenanceWindow
      is now a pure existence-check no-op, matching StartInputDevice/
      StopInputDevice's existing pattern (StartInputDeviceMaintenanceWindowOutput
      carries no fields on the real wire either).
      gopherstack-tp8x (2026-08-21), FIXED: DescribeInputDeviceThumbnail was
      a header-vs-body confusion, not a key-casing bug -- confirmed against
      awsRestjson1_deserializeOpHttpBindingsDescribeInputDeviceThumbnailOutput,
      which binds ContentType/ContentLength/ETag/LastModified to the
      Content-Type/Content-Length/ETag/Last-Modified HTTP response headers,
      and awsRestjson1_deserializeOpDocumentDescribeInputDeviceThumbnailOutput,
      which sets Body directly from the raw response body (no JSON
      unwrapping). The handler wrote a JSON object
      {"ContentType":"image/jpeg","ContentLength":0} with none of those as
      real headers, so a real client's typed ContentType/ContentLength
      fields decoded as zero values regardless of what was "sent". Fixed to
      set real ETag/Last-Modified headers and return the body via c.Blob
      with a real Content-Type header -- same convention as
      iotdataplane's GetThingShadow. No real thumbnail image is captured by
      this backend (body is empty bytes); only the wire *shape* is fixed.
      Locked by TestDescribeInputDeviceThumbnail_HeadersNotBody_RealClient.
      NOTE: gopherstack-tp8x's filing cited apigateway's GetSdk as an
      existing correct example of this same header/raw-body convention --
      checked while fixing this, and that citation is wrong: apigateway's
      GetSdk (handler_sdk.go) still JSON-marshals
      {"contentType","contentDisposition","body"} through the same
      dispatch()/c.JSONBlob() path as every other apigateway op, with no
      header-setting or raw-body special case anywhere in the dispatch
      chain (handler.go's dispatch/dispatchAndRespond). apigateway's GetSdk
      has the same header-vs-body bug this entry just fixed for medialive;
      it was NOT touched by this pass (out of gopherstack-tp8x's scope) and
      is now a known, confirmed gap for apigateway -- see that service's
      PARITY.md / file a follow-up before trusting the old "already
      correct" note again.
  Cluster:
    status: ok
    note: >
      FIXED this pass. clusterOutput's "Tags"/"Arn"/"Id"/"Name"/
      "ClusterType"/"InstanceRoleArn"/"State" were PascalCase; fixed to
      lowerCamel ("arn"/"id"/"name"/"clusterType"/"instanceRoleArn"/
      "state"). The real DescribeClusterOutput/CreateClusterOutput/
      UpdateClusterOutput have NO "tags" field at all (verified against the
      SDK deserializer) even though CreateClusterInput accepts tags --
      dropped Tags from clusterOutput entirely; Cluster tags now only
      surface via ListTagsForResource, matching AWS. Added the wire's
      "channelIds" field (real API field gopherstack never emitted at all)
      as a derived empty list -- gopherstack doesn't track cluster-channel
      association, tracked as a residual gap below. Request-body parsing
      fixed too: handleCreateCluster read "ClusterType"/"InstanceRoleArn"
      (PascalCase, never matches a real client's lowerCamel body) --
      CreateCluster silently ignored the caller's clusterType/
      instanceRoleArn before this fix. ListClusters' summary map and
      wrapper ("Clusters" -> "clusters") fixed too. ListClusterAlerts'
      synthetic alert also had entirely wrong field names ("AlertCode"/
      "AlertMessage"/"SetTime"/"ClearedTime" -- none of which exist on the
      real ClusterAlert shape); rewritten to the real field names (id/
      alertType/message/state/setTimestamp).
      SWEEP 5: independently field-diffed DescribeClusterOutput/
      CreateClusterInput/UpdateClusterInput against the SDK deserializer/
      serializers and found TWO more real gaps sweep 4 missed. (1)
      "networkSettings" (types.ClusterNetworkSettings: defaultRoute +
      interfaceMappings[].logicalInterfaceName/networkId) was not tracked
      AT ALL -- CreateCluster/UpdateCluster silently discarded a caller's
      networkSettings. Added Cluster.NetworkSettings, wired into
      Create/UpdateCluster (UpdateCluster only overwrites it when the
      caller includes the key, matching UpdateClusterInput's "include this
      parameter only if you want to change it" semantics) and
      Describe/Create/Update/List output ("networkSettings" omitted
      entirely when never configured, matching a real Cluster). (2)
      "channelIds" is now a REAL derived value (sorted Channel IDs whose
      AnywhereSettings.ClusterID matches this cluster -- see Channel's
      sweep-5 note above) instead of the hardcoded `[]string{}` sweep 4
      left in place; the residual gap this closes is listed as fixed
      below.
  Node:
    status: ok
    note: >
      FIXED this pass, same bug class as Cluster. nodeOutput's PascalCase
      keys fixed to lowerCamel ("arn"/"id"/"name"/"clusterId"/"role"/
      "state"/"connectionState"). Real DescribeNodeOutput/CreateNodeOutput/
      UpdateNodeOutput/UpdateNodeStateOutput have NO "tags" field either
      (same as Cluster) -- dropped. Added the wire's "channelPlacementGroups"
      field as a derived empty list (gopherstack doesn't track it; residual
      gap below). Request-body parsing fixed: handleCreateNode/
      handleUpdateNode read "Role" (should be "role"; CreateNodeInput's
      real field is lowerCamel), handleUpdateNodeState read "State" (should
      be "state"). ListNodes summary map and wrapper ("Nodes" -> "nodes")
      fixed too.
      SWEEP 5: "channelPlacementGroups" is now a REAL derived value (sorted
      ChannelPlacementGroup IDs, within this node's cluster, whose Nodes
      list contains this node's ID) instead of the hardcoded `[]string{}`
      sweep 4 left in place. Also fixed a leak: DeleteNode never removed
      the node's b.tags[ARN] entry (Node isn't in taggableResourceTags'
      fast path, so its tags live in the legacy per-ARN store) -- ghost
      row left behind on every delete. Fixed by clearing it in DeleteNode.
  ChannelPlacementGroup:
    status: ok
    note: >
      FIXED this pass, same bug class. toChannelPlacementGroupOutput's
      "ClusterId"/"Channels"/"Nodes" fixed to lowerCamel ("clusterId"/
      "channels"/"nodes"), verified against the real
      DescribeChannelPlacementGroupOutput shape (arn/channels/clusterId/id/
      name/nodes/state -- no tags field, matches gopherstack's model which
      never had one). Request-body parsing fixed: handleCreate/
      UpdateChannelPlacementGroup read "Nodes" (should be "nodes"). List
      wrapper ("ChannelPlacementGroups" -> "channelPlacementGroups") fixed.
      SWEEP 5: "channels" is now a REAL derived value (sorted Channel IDs
      whose AnywhereSettings.ChannelPlacementGroupID matches this group)
      instead of a `Channels []string` struct field that was always
      initialized to `[]string{}` and never updated on channel attach --
      removed the dead persisted field entirely, replaced with a live
      derivation (channelIDsForPlacementGroup). Also fixed two leaks: (1)
      DeleteChannelPlacementGroup never removed the group's b.tags[ARN]
      entry (same class of bug as Node, above); (2) DeleteCluster never
      cascade-deleted its ChannelPlacementGroups at all -- unlike Nodes
      (embedded directly in storedCluster.Nodes, removed automatically
      with their parent), ChannelPlacementGroup lives in its own top-level
      table keyed by "clusterID/groupID", so nothing removed it (or its
      tags) when the owning Cluster was deleted; every cluster
      create+delete cycle with a placement group left a permanently
      orphaned row. Fixed via cascadeDeleteChannelPlacementGroups, called
      from DeleteCluster.
  SignalMap:
    status: ok
    note: >
      FIXED this pass. toSignalMapOutput's PascalCase keys fixed to
      lowerCamel ("discoveryEntryPointArn"/"status"/
      "monitorDeploymentStatus"/"cloudWatchAlarmTemplateGroupIds"/
      "eventBridgeRuleTemplateGroupIds"). Added "createdAt"/"modifiedAt"
      (previously missing entirely from the model) -- confirmed ISO8601
      string wire form via smithytime.ParseDateTime in the SDK deserializer
      (NOT epoch-seconds); new SignalMap.CreatedAt/ModifiedAt time.Time
      fields, stamped on Create/StartUpdateSignalMap/
      StartMonitorDeployment/StartDeleteMonitorDeployment, rendered via
      new `formatISO8601` helper (RFC3339, omits empty on zero-value).
      Request-body parsing fixed: handleCreateSignalMap/
      handleStartUpdateSignalMap read "DiscoveryEntryPointArn"/
      "CloudWatchAlarmTemplateGroupIdentifiers"/
      "EventBridgeRuleTemplateGroupIdentifiers" (all PascalCase -- verified
      the real CreateSignalMapInput/StartUpdateSignalMapInput serializers
      use lowerCamel). ListSignalMaps wrapper ("SignalMaps" ->
      "signalMaps") fixed.
      SWEEP 5: fixed a leak -- DeleteSignalMap never removed the signal
      map's b.tags[ARN] entry (SignalMap isn't in taggableResourceTags'
      fast path). Same fix applied to every other family below outside
      that fast path (Reservation, Network, SdiSource,
      CloudWatchAlarmTemplate(Group), EventBridgeRuleTemplate(Group)) --
      noted once here, not repeated per family below.
      gopherstack-uult (this pass): ListSignalMaps reused toSignalMapOutput
      unscoped, leaking discoveryEntryPointArn/
      cloudWatchAlarmTemplateGroupIds/eventBridgeRuleTemplateGroupIds --
      none of which types.SignalMapSummary declares. Fixed with a dedicated
      toSignalMapSummary. Note the trap here: types.SignalMapSummary DOES
      declare "tags" (verified against deserializers.go's
      awsRestjson1_deserializeDocumentSignalMapSummary), unlike every other
      List-vs-Get pair fixed in this sweep -- tags was correctly kept on the
      summary, not dropped.
  CloudWatchAlarmTemplateGroup:
    status: ok
    note: >
      FIXED this pass. toCWAlarmTemplateGroupOutput's keys fixed to
      lowerCamel. Added "createdAt"/"modifiedAt" (ISO8601 strings, same
      confirmation as SignalMap). Note: the real Get/Create/Update
      responses have NO "templateCount" field -- only the List response's
      Summary shape does (verified against the SDK deserializer); List
      still reuses this same output function and so is still missing
      templateCount (tracked as a residual gap below -- would need a new
      backend method to count templates per group). List wrapper
      ("CloudWatchAlarmTemplateGroups" -> "cloudWatchAlarmTemplateGroups")
      fixed.
      SWEEP 5: FIXED the templateCount gap. Added
      CloudWatchAlarmTemplateGroupSummary (embeds
      CloudWatchAlarmTemplateGroup + TemplateCount int32),
      countCWAlarmTemplatesForGroup (live count, O(n) scan of
      cwAlarmTemplates filtered by GroupID), and
      toCWAlarmTemplateGroupSummaryOutput (Get/Create/Update still use
      toCWAlarmTemplateGroupOutput, which correctly has no templateCount
      key; only List's handler now calls the Summary variant).
      ListCloudWatchAlarmTemplateGroups' return type changed from
      []*CloudWatchAlarmTemplateGroup to
      []*CloudWatchAlarmTemplateGroupSummary.
  CloudWatchAlarmTemplate:
    status: ok
    note: >
      FIXED this pass. toCWAlarmTemplateOutput's keys fixed to lowerCamel.
      Dropped "groupIdentifier" and "namespace" from the output entirely --
      neither is a real field on GetCloudWatchAlarmTemplateOutput/
      CreateCloudWatchAlarmTemplateOutput/UpdateCloudWatchAlarmTemplateOutput
      (verified against the SDK deserializer: only "groupId" is returned,
      no "namespace" at all). Added "createdAt"/"modifiedAt" (ISO8601).
      Request-body parsing fixed: extractCWAlarmTemplateFields read
      "GroupIdentifier"/"MetricName"/"Namespace"/"Statistic"/
      "ComparisonOperator"/"TargetResourceType"/"TreatMissingData"/
      "Threshold"/"EvaluationPeriods"/"DatapointsToAlarm"/"Period" (all
      PascalCase) -- CreateCloudWatchAlarmTemplate/
      UpdateCloudWatchAlarmTemplate silently ignored every one of these
      caller-supplied fields before this fix. List wrapper
      ("CloudWatchAlarmTemplates" -> "cloudWatchAlarmTemplates") fixed.
  EventBridgeRuleTemplateGroup:
    status: ok
    note: >
      FIXED this pass, same shape as CloudWatchAlarmTemplateGroup (no
      templateCount on Get/Create/Update, only on List's Summary shape --
      same residual gap, see CloudWatchAlarmTemplateGroup). Added
      "createdAt"/"modifiedAt". List wrapper
      ("EventBridgeRuleTemplateGroups" -> "eventBridgeRuleTemplateGroups")
      fixed.
      SWEEP 5: FIXED the templateCount gap, same shape as
      CloudWatchAlarmTemplateGroup above -- added
      EventBridgeRuleTemplateGroupSummary, countEBRuleTemplatesForGroup,
      toEBRuleTemplateGroupSummaryOutput; List's return type changed to
      []*EventBridgeRuleTemplateGroupSummary.
  EventBridgeRuleTemplate:
    status: ok
    note: >
      FIXED this pass. toEBRuleTemplateOutput's keys fixed to lowerCamel
      ("groupId"/"eventType"/"eventTargets"). Dropped "groupIdentifier" --
      not a real field on this shape (only "groupId" is returned, verified
      against the SDK deserializer). Added "createdAt"/"modifiedAt". Note:
      the real List response's Summary shape returns "eventTargetCount"
      instead of the full "eventTargets" array -- gopherstack's List still
      reuses the Get/Create/Update shape and so over-returns the full
      target list instead of a count (tracked as a residual gap below).
      Request-body parsing fixed: handleCreate/UpdateEBRuleTemplate read
      "GroupIdentifier"/"EventType" (PascalCase) and extractEBTargets read
      "EventTargets" (PascalCase) -- all silently ignored caller input
      before this fix. List wrapper ("EventBridgeRuleTemplates" ->
      "eventBridgeRuleTemplates") fixed.
      SWEEP 5: FIXED the eventTargetCount gap. Added
      EventBridgeRuleTemplateSummary (embeds EventBridgeRuleTemplate +
      EventTargetCount int32), toEBRuleTemplateSummaryOutput (emits
      "eventTargetCount", omits "eventTargets" entirely -- matching the
      real Summary shape, which has no eventTargets field at all).
      Get/Create/Update still return the full eventTargets array via
      toEBRuleTemplateOutput, unchanged. List's return type changed to
      []*EventBridgeRuleTemplateSummary.
  Offering:
    status: ok
    note: >
      FIXED this pass. toOfferingOutput's keys fixed to lowerCamel. Added
      the wire's "region" field (real DescribeOfferingOutput/Offering HAS
      a region field; gopherstack's Offering model never tracked it) --
      new Offering.Region field, populated in seedOfferings from the
      backend's configured region. Confirmed the real shape has NO "name"
      and NO "tags" field (gopherstack's model never had either, so nothing
      to drop). List wrapper ("Offerings" -> "offerings") fixed.
  Reservation:
    status: ok
    note: >
      FIXED this pass, same bug class. toReservationOutput's keys fixed to
      lowerCamel. PurchaseOffering's response wrapper ("Reservation" ->
      "reservation") fixed. UpdateReservation's response was NOT wrapped at
      all before this fix -- the real UpdateReservationOutput wraps in
      "reservation" (verified against the SDK deserializer) while
      DescribeReservationOutput/DeleteReservationOutput do NOT wrap (bare
      top-level fields) -- fixed handleUpdateReservation to wrap, left
      Describe/Delete unwrapped (both already matched). Request-body
      parsing fixed: handlePurchaseOffering read "Count" (should be
      "count"). List wrapper ("Reservations" -> "reservations") fixed. Not
      added: the real "renewalSettings" field (gopherstack's Reservation
      model doesn't track renewal settings at all; tracked as a residual
      gap below since it's a new field, not a casing fix).
      SWEEP 5: FIXED the renewalSettings gap. Added
      Reservation.RenewalSettings (AutomaticRenewal/RenewalCount, wire keys
      "renewalSettings.automaticRenewal"/"renewalSettings.renewalCount" --
      verified against awsRestjson1_serializeDocumentRenewalSettings),
      wired into PurchaseOffering/UpdateReservation request parsing
      (UpdateReservation only overwrites it when the caller includes the
      key) and Describe/Purchase/Update/List output ("renewalSettings"
      omitted entirely when never configured, matching a real
      never-configured Reservation). Also fixed a leak: DeleteReservation
      never removed the reservation's b.tags[ARN] entry.
  Network:
    status: ok
    note: >
      FIXED this pass. toNetworkOutput's keys fixed to lowerCamel
      ("associatedClusterIds"/"ipPools"/"routes"). The nested IPPool/Route
      Go structs' own json tags were also PascalCase ("Cidr"/"Gateway") --
      fixed to lowerCamel since they're marshaled directly as nested
      objects. Request-body parsing fixed: extractIPPools/extractRoutes
      read "IpPools"/"Routes"/"Cidr"/"Gateway" (all PascalCase) -- silently
      ignored caller input before this fix. List wrapper ("Networks" ->
      "networks") fixed.
  SdiSource:
    status: ok
    note: >
      FIXED this pass. toSdiSourceOutput's keys fixed to lowerCamel
      ("type"/"mode"/"inputs"); "sdiSource" wrapper key already correct via
      the shared `keySdiSource` constant (now flipped from "SdiSource" to
      "sdiSource"). Request-body parsing fixed: handleCreate/
      UpdateSdiSource read "Type"/"Mode" (PascalCase). List wrapper
      ("SdiSources" -> "sdiSources") fixed.
  Batch:
    status: ok
    note: >
      FIXED this pass -- both the wire-casing gap AND the two non-casing
      bugs PARITY.md previously flagged. (1) CASING: toBatchResultOutput's
      wrapper ("Successful"/"Failed" -> "successful"/"failed") and item
      keys ("Code" -> "code", added "message") fixed. BatchUpdateSchedule's
      "Creates"/"Deletes"/"ScheduleActions"/"ActionName"/"ActionNames" all
      fixed to lowerCamel (request AND response). (2) REQUEST-SHAPE BUG
      (fixed): verified against api_op_BatchStart.go/api_op_BatchStop.go/
      api_op_BatchDelete.go and their serializers -- BatchStartInput/
      BatchStopInput have ONLY ChannelIds+MultiplexIds (wire:
      channelIds/multiplexIds), NO InputIds field at all.
      BatchStart/BatchStop's Go signatures changed from
      `(channelIDs, inputIDs, multiplexIDs []string)` to
      `(channelIDs, multiplexIDs []string)` -- the dead inputIDs parameter
      is gone, not just ignored. BatchDeleteInput DOES have all four:
      ChannelIds+InputIds+MultiplexIds+InputSecurityGroupIds (wire:
      channelIds/inputIds/multiplexIds/inputSecurityGroupIds) --
      handleBatchDelete now parses "inputSecurityGroupIds" (previously
      never parsed at all: a real client batch-deleting an
      InputSecurityGroup was silently ignored) and
      InMemoryBackend.BatchDelete gained a 4th parameter + a new
      `batchDeleteInputSecurityGroups` helper. New test
      TestBatch_DeleteInputSecurityGroups proves the fix end-to-end.
  Schedule:
    status: ok
    note: >
      FIXED this pass. DescribeSchedule's wrapper (keyScheduleActions:
      "ScheduleActions" -> "scheduleActions") and item key (keyActionName:
      "ActionName" -> "actionName") fixed via the shared constants (safe:
      grepped all call sites, all Batch/Schedule family, all in scope this
      pass).
  Alerts:
    status: ok
    note: >
      FIXED this pass. ListAlerts/ListMultiplexAlerts/ListClusterAlerts'
      wrapper (keyAlerts: "Alerts" -> "alerts") fixed via the shared
      constant. ListAlerts/ListMultiplexAlerts always return an empty list
      in this emulator (unchanged, no per-item casing to fix). See Cluster
      above for the ListClusterAlerts synthetic-alert field-name fix.
  AccountConfiguration:
    status: ok
    note: >
      FIXED this pass. Describe/UpdateAccountConfiguration's wrapper
      ("AccountConfiguration" -> "accountConfiguration") and inner key
      ("KmsKeyId" -> "kmsKeyId") fixed, both response AND request-body
      parsing (handleUpdateAccountConfiguration read the same wrong keys).
  Versions:
    status: ok
    note: >
      FIXED this pass. ListVersions' wrapper ("Versions" -> "versions") and
      item keys ("Version"/"ExpirationDate" -> "version"/"expirationDate")
      fixed. Also fixed a latent decode-breaking bug: expirationDate is
      __timestampIso8601 (smithytime.ParseDateTime) on the real wire, and
      gopherstack always emitted it as "" (channelEngineVersion has no
      real expiration data) -- a real SDK client would fail to parse ""
      as a timestamp. Now omits the key entirely when empty instead of
      emitting an unparseable "".
  Offering-Reservation-shared:
    status: ok
    note: See Offering and Reservation above.

# Families out of scope this pass (nothing left deferred at the family
# level -- every family named in sweep 4/5 is `ok`). Remaining work is
# residual, sub-family gaps, listed below.
deferred: []

# The 2 concrete gaps sweep 5 left open (Channel's 12-of-17-member gap and
# the channelPlacementGroupId validation gap) are now CLOSED by sweep 6
# (gopherstack-jb9i) -- see Channel's SWEEP 6 note above for exactly what
# changed and how each was verified against the SDK. What's left below is
# either newly-discovered-and-closed (kept here only as a paper trail) or
# genuinely out of scope for this pass.
gaps:
  - Channel's EncoderSettings is modeled to a deliberately bounded depth (sweep 6,
    gopherstack-jb9i; extended by gopherstack-sthr across two sub-passes, then gopherstack-hj9n,
    then gopherstack-1szb). See Channel's note above for the full list of what IS modeled:
    AvailConfiguration/ColorCorrectionSettings/MotionGraphicsConfiguration/NielsenConfiguration
    (gopherstack-sthr pass 1 -- none turned out to be a large per-format union, each is a small
    flat struct or a small tagged union); AudioDescription's CodecSettings/
    AudioNormalizationSettings/AudioWatermarkingSettings/RemixSettings/AudioDashRoles/
    DvbDashAccessibility (gopherstack-sthr pass 2 -- the AudioCodecSettings union verified as 7
    variants of flat scalar structs, not the ~20 the bd issue title estimated);
    OutputGroup.OutputGroupSettings + Output.OutputSettings, modeled together (gopherstack-hj9n --
    11 variants each, down through every nested container/CDN/stream sub-union: M2tsSettings,
    MultiplexM2tsSettings, HlsSettings, HlsCdnSettings, KeyProviderSettings, ArchiveCdnSettings,
    FrameCaptureCdnSettings, M3u8Settings, MediaPackageV2GroupSettings/
    MediaPackageV2DestinationSettings); CaptionDescription.DestinationSettings + CaptionDashRoles
    (gopherstack-1szb, first sub-pass -- types.CaptionDestinationSettings is 13 variants, not the
    12 the bd issue counted: 8 empty-marker structs, Ttml/Webvtt single-field, EbuTtD 6 fields,
    BurnIn/DvbSub 18 fields each, not 19 as originally estimated); and
    VideoDescription.CodecSettings (gopherstack-1szb, final sub-pass -- types.VideoCodecSettings,
    5 variants, measured at Av1Settings 24 fields, H264Settings 44, H265Settings 42,
    Mpeg2Settings 17, FrameCaptureSettings 3, all sharing TimecodeBurninSettings; H264/H265's
    FilterSettings sub-union is identical between the two and shares one wire struct). This
    closes the last EncoderSettings union -- no gap remains in this family at the union level.
    (bd: gopherstack-jb9i closed the 12-of-17-member gap; gopherstack-sthr closed
    AvailConfiguration/ColorCorrectionSettings/MotionGraphicsConfiguration/NielsenConfiguration
    and, in a second sub-pass, AudioDescription's codec/normalization/watermarking/remix/
    dash-role/accessibility fields; gopherstack-hj9n closed OutputGroupSettings/OutputSettings
    together per its explicit ordering instruction; gopherstack-1szb closed
    CaptionDestinationSettings and, in a follow-up sub-pass once measured and confirmed
    tractable, VideoCodecSettings -- the union this whole gap entry originally tracked.)
  - InputAttachment.InputSettings is now modeled in full (gopherstack-sthr, this pass) -- see
    Channel's note above. InputAttachmentName/InputId/LogicalInterfaceNames/
    AutomaticInputFailoverSettings (including all 3 failover-condition variants) were already
    modeled (sweep 6). No open gap remains in this family.
  - Channel.Vpc's response-side availabilityZones/networkInterfaceIds (types.
    VpcOutputSettingsDescription) are always omitted -- MediaLive computes them from a real
    VPC/ENI integration gopherstack does not have. The request-side subnetIds/
    publicAddressAllocationIds/securityGroupIds ARE modeled and echoed back (sweep 6).
  - Deep state/error-code audit of Cluster, Node, SignalMap, Reservation/Offering purchase
    flow, Batch semantics beyond the wire-casing scope of sweep 4 and the association/
    leak/new-field fixes sweep 5 made was not re-performed (route matching for all of them was
    verified correct in sweep 4; op-by-op state-machine correctness beyond what these two
    passes touched was not re-verified). UPDATE 2026-08-23: this gap is what prompted the
    Reservation/Offering request-side audit below ("every List operation ignored the client's
    maxResults/nextToken"), which found and fixed the same real bug across 20 List handlers
    spanning every family in the service (not just Reservation/Offering) but did not attempt
    the full state/error-code re-audit this entry originally called for; Cluster/Node/
    SignalMap/Batch semantics and DeleteReservation's hard-delete-vs-DELETED-state question
    (see the same dated entry) remain open.

  - "Constraining-parameter sweep (wrapper-key campaign, 2026-08-29): six real
    never-applied-constraint bugs found and fixed, all confirmed with a real
    aws-sdk-go-v2 client test that failed against the unfixed handler first.
    (1) ListClusterAlerts never read StateFilter (SET/CLEARED/ALL) -- the
    synthetic \"cluster-not-ready\" alert (always state SET) was returned for
    ANY filter value, so a client asking for CLEARED alerts wrongly got the
    SET one back; now stateFilter==\"CLEARED\" excludes it.
    (2) ListReservations never read Codec/MaximumBitrate/MaximumFramerate/
    Resolution/ResourceType/SpecialFeature/VideoQuality -- an account can
    purchase an unbounded number of reservations (see the pagination test's
    25-reservation setup), so unlike ListOfferings' fixed 3-item catalog
    (left unfixed -- see below) this was the \"unbounded counts\" case that
    must honor its filters, not the \"at most a few values\" restraint case;
    now filtered via ReservationFilter (reservations.go) against each
    reservation's inherited ResourceSpecification. ChannelClass is NOT
    filterable -- neither Offering nor Reservation tracks it anywhere in
    this backend, a genuine structural gap, disclosed rather than faked.
    (3) ListCloudWatchAlarmTemplates/ListEventBridgeRuleTemplates never read
    GroupIdentifier (resolved via the same findCWAlarmTemplateGroup/
    findEBRuleTemplateGroup ID/ARN/name lookup Create already uses) or
    SignalMapIdentifier (a signal map's own cloudWatchAlarmTemplateGroupIds/
    eventBridgeRuleTemplateGroupIds lists, both AND-combinable with
    GroupIdentifier).
    (4) ListCloudWatchAlarmTemplateGroups/ListEventBridgeRuleTemplateGroups
    never read SignalMapIdentifier -- same signal-map-list match, shared via
    the new generic listTemplateGroups (cloudwatch_alarm_templates.go).
    (5) ListSignalMaps never read CloudWatchAlarmTemplateGroupIdentifier/
    EventBridgeRuleTemplateGroupIdentifier -- the reverse direction of (4),
    filtering signal maps down to those referencing a given group.
    (6) ListInputDeviceTransfers echoed back whatever transferType
    (OUTGOING/INCOMING) the client queried on every pending transfer,
    regardless of its real direction -- TransferInputDevice is the only way
    this backend ever creates a pending transfer, and it always makes THIS
    account the source (no path exists for another account to initiate a
    transfer targeting this one), so every pending transfer is inherently
    OUTGOING; querying INCOMING now correctly returns empty instead of the
    same devices relabeled. This also corrected an existing test
    (TestHandlerListInputDeviceTransfers's \"incoming transfers\" case) that
    asserted the bug's own wrong output (wantCount: 2) as correct.
    Left as disclosed restraint, not fixed: ListOfferings' 10 filter params
    (ChannelClass/ChannelConfiguration/Codec/Duration/MaximumBitrate/
    MaximumFramerate/Resolution/ResourceType/SpecialFeature/VideoQuality) --
    seedOfferings is a fixed 3-item catalog (store.go), squarely the \"at
    most one to three values can ever exist\" case filtering would not
    meaningfully change; ChannelConfiguration additionally requires deriving
    compatibility from an existing channel's configuration, a distinct
    feature with no backing logic here. medialive's Scope filter (LOCAL vs
    AWS_MANAGED on the CW/EB template-group List ops) was also left
    unimplemented: it is a plain *string in the pinned SDK with no typed
    enum anywhere in the module (grepped types/enums.go and the whole SDK
    package for AWS_MANAGED/LOCAL -- zero hits), so its exact wire values
    are asserted only in a prose doc comment; implementing a filter against
    an unverified literal risks the wrong-vocabulary bug class more than
    leaving it a documented gap, since this backend has zero AWS-managed
    groups to ever wrongly include regardless."

leaks: {status: clean, note: "No goroutines/janitors in this service (re-confirmed sweep 5: no `go func`/time.NewTicker/time.AfterFunc/context.WithCancel anywhere in non-test files). Two real leaks found and fixed this pass: (1) b.tags[ARN] rows were never removed on delete for every resource family outside the Channel/Input/InputSecurityGroup/Multiplex/InputDevice fast path (taggableResourceTags) -- Cluster/Node/SignalMap/CloudWatchAlarmTemplate(Group)/EventBridgeRuleTemplate(Group)/Reservation/Network/SdiSource/ChannelPlacementGroup all now clear their b.tags entry in their respective Delete method; regression-tested via TestTags_LegacyStoreClearedOnDelete. (2) DeleteCluster never cascade-deleted its ChannelPlacementGroups -- unlike Nodes (embedded in storedCluster.Nodes, removed automatically with their parent), ChannelPlacementGroup lives in its own top-level table keyed by \"clusterID/groupID\"; fixed via cascadeDeleteChannelPlacementGroups, regression-tested via TestChannelPlacementGroup_CascadeDeletedWithCluster. Every b.mu.Lock/RLock call site was re-verified this pass to have an immediately-following `defer b.mu.Unlock()`/`RUnlock()` (125 call sites, no exceptions)."}

---

## Notes

**Wire protocol**: REST-JSON1 (`/prod/...` paths, JSON bodies, HTTP verbs GET/POST/PUT/DELETE/PATCH map 1:1 to List/Create/Update/Delete/Update-partial). No XML anywhere in this service.

**The casing bug, precisely**: aws-sdk-go-v2's restjson1 deserializers decode the
HTTP body into a `map[string]interface{}` via `encoding/json`, then dispatch
per-field with a Go `switch key { case "arn": ... }` -- an *exact string
match*, not a case-insensitive one (unlike a direct
`json.Unmarshal(body, &typedStruct)`, which Go's encoding/json *does*
case-fold). A response field emitted as `"Arn"` never matches `case "arn":`
and silently falls through to the `default: _, _ = key, value` branch,
leaving that field at its Go zero value in the decoded SDK type. Request
bodies have the mirror-image bug: a handler reading `body["GroupIdentifier"]`
when a real client always sends `"groupIdentifier"` silently no-ops on every
real caller's input. This pass (sweep 4) closed this bug class for every
remaining family in the service -- see the per-family notes above for the
specific keys fixed in each.

**Shared constants, now fully unified**: as of this pass, `keyArn`/`keyID`/
`keyName`/`keyState`/`keyTags`/`keyDescription`/`keyAlerts`/`keyActionName`/
`keyScheduleActions`/`keySdiSource` are ALL lowerCamel ("arn"/"id"/"name"/
"state"/"tags"/"description"/"alerts"/"actionName"/"scheduleActions"/
"sdiSource"), and the previously-separate `wireArn`/`wireID`/`wireName`/
`wireState` constants (introduced in the prior pass for Channel/Multiplex/
Input's already-correct casing) were deleted and their call sites repointed
at the now-identical `keyArn`/`keyID`/`keyName`/`keyState` -- there is no
longer a "these two constant sets differ" trap for future readers to fall
into, since every family in the service uses the same lowerCamel casing.
`keyMessage` ("Message", PascalCase, used only for the shared `respondErr`
error-response body) was deliberately left untouched -- error-response
casing was out of scope for this pass and no family's fix depended on it.

**Timestamp wire form, confirmed empirically this pass**: SignalMap/
CloudWatchAlarmTemplate(Group)/EventBridgeRuleTemplate(Group)'s
createdAt/modifiedAt, and ChannelAlert/ClusterAlert/MultiplexAlert's
setTimestamp/clearedTimestamp, and ListVersions' expirationDate are all
`__timestampIso8601`, deserialized via `smithytime.ParseDateTime` (grepped
directly in aws-sdk-go-v2/service/medialive@v1.101.4's deserializers.go for
every shape touched this pass) -- an ISO8601/RFC3339 string, NOT epoch
seconds (`pkgs/awstime.Epoch` does NOT apply here; that helper is for
services using the unixTimestamp wire form, which medialive's restjson1
protocol does not default to for these specific shapes). New helper
`formatISO8601` (handler.go) renders `time.Time` via `time.RFC3339`,
omitting the key when the time is zero-valued (a real SDK client would fail
to parse `""` as a timestamp -- this bit ListVersions' expirationDate before
this pass's fix).

**PipelinesRunningCount / ProgramCount semantics** (Channel/Multiplex,
prior pass, unchanged): derived, not persisted. Computed at read time from
State (+ ChannelClass for Channel) or len(Programs) (for
Multiplex.ProgramCount).

**ARN suffix convention** (for anyone extending `taggableResourceTags`):
each `*ARN(id string) string` builder in backend.go appends
`"<resourceType>:<id>"` as the ARN's resource segment. `taggableResourceTags`
does an O(n) linear scan of each candidate table's `.All()` comparing `.ARN`
fields directly.

**Derived-association pattern (sweep 5)**: Cluster.ChannelIds,
ChannelPlacementGroup.Channels, and Node.ChannelPlacementGroups are computed
live at read time (Create/Describe/Update/Delete/List), never persisted --
same pattern as PipelinesRunningCount/ProgramCount above. Each is an O(n)
scan of the relevant table filtered by a foreign-key-style field
(Channel.AnywhereSettings.ClusterID/ChannelPlacementGroupID,
ChannelPlacementGroup.Nodes), sorted for deterministic output. The three
helpers -- `channelIDsForCluster`, `channelIDsForPlacementGroup`,
`channelPlacementGroupIDsForNode` -- all require the caller to already hold
`b.mu` (Lock or RLock); none of them takes the lock themselves, since every
call site is already inside a locked backend method.

**AnywhereSettings / NetworkSettings wire shape (sweep 5)**: both are
optional nested objects that a real, non-Anywhere Channel/never-configured
Cluster omits entirely from its JSON response (verified against the SDK
deserializer: `*types.AnywhereSettings`/`*types.ClusterNetworkSettings`, nil
until configured) -- NOT emitted as `{}` or with empty-string/zero-value
subfields. `ChannelAnywhereSettings.hasAnywhereSettings()` and
`ClusterNetworkSettings.hasNetworkSettings()` gate this: the output-struct
pointer is nil (`omitempty`) unless at least one subfield is non-zero.
UpdateChannel/UpdateCluster/UpdateReservation all follow the same
"has-the-key-at-all" convention for their respective optional nested
objects (anywhereSettings/networkSettings/renewalSettings): the handler's
`extractX` function returns `(zeroValue, false)` when the request body
omits the key entirely, and the backend method only overwrites the stored
value when the second return is `true` -- an explicit `{}` in the request
body IS treated as "change it to empty", only a fully-omitted key preserves
the existing value. This mirrors each field's real Update*Input doc comment
("include this parameter only if you want to change it").

**SDK version drift (found by gopherstack-sthr, closed by gopherstack-u8my's
pin-correction pass)**: `sdk_module` above and every `v1.97.2` citation
elsewhere in this file previously reflected the version audited as of sweep
6, while go.mod pinned `v1.101.4` (4 minor versions ahead). gopherstack-sthr's
AvailConfiguration/ColorCorrectionSettings/MotionGraphicsConfiguration/
NielsenConfiguration addition (handler_channels_encoder.go) was already
verified against the pinned v1.101.4. This pass re-verified the remaining
citations (RouteMatcher's 123/123 op+path diff, the Channel top-level member
field-diff, and the ISO8601-vs-epoch timestamp grep) directly against
v1.101.4: `serializers.go` has zero `SplitURI`/method diffs, `types/errors.go`
is byte-identical, and `deserializers.go` has zero diffs touching any of the
timestamp shapes named above between v1.97.2 and v1.101.4 -- op count is
123/123 in both. All citations now correctly read v1.101.4, and `sdk_module`
above is corrected to match.

## 2026-08-22 gopherstack-wlo1: error envelope is wire shape too

Two dispatch-failure sites in `handleREST` -- the "invalid JSON body" 400
and the "unknown operation" 404 -- wrote a JSON body via `keyMessage` alone,
without ever setting `amznErrorTypeHeader` (X-Amzn-Errortype). The genuine
backend-error path (`respondErr`, which calls `errType(err)`) already set
this header correctly, so this affected only these two malformed/dispatch
paths, not every operation. aws-sdk-go-v2/service/medialive@v1.101.4's
`awsRestjson1_deserializeOpError*` functions read X-Amzn-ErrorType first,
falling back to a body `code`/`__type` field only if absent (confirmed via
`deserializeOpErrorDescribeChannel`, which models `NotFoundException`, and
`deserializeOpErrorCreateChannel`, which models `BadRequestException` --
both in deserializers.go). With neither header nor body type set, both
paths decoded client-side as `smithy.GenericAPIError{Code:"UnknownError"}`.

Fixed by setting `amznErrorTypeHeader` to `"BadRequestException"` (matches
`errType`'s `awserr.ErrInvalidParameter` mapping) before the malformed-body
response, and to `"NotFoundException"` (matches `errType`'s
`awserr.ErrNotFound` mapping, and the 404 status already in use) before the
unknown-operation response.

Proof: since no real SDK operation can construct a malformed body or an
unrecognised route on its own, `handler_error_type_test.go` adds two tests
that drive a genuine `medialivesdk.Client` through smithy middleware that
corrupts the outgoing request after normal serialization/signing --
`TestCreateChannel_MalformedBodySurfacesBadRequestException` (corrupts the
body) and `TestCreateChannel_UnrecognisedRouteSurfacesNotFoundException`
(rewrites the path to one `classifyPath` doesn't recognise, keeping the
`/prod/` prefix `RouteMatcher` requires). Both confirmed failing against
the unfixed `handler.go` (asserted "UnknownError" instead of the intended
code) via hand-revert, then restored byte-identical (md5sum-verified).
Same bug class as the sibling mediatailor (f41d5b42f) and vpclattice
(gopherstack-wlo1, this session) services, and the s3control/iot instances
that opened gopherstack-wlo1.

## 2026-08-22: DeleteInput must stay describable as DELETED, not vanish

CI's terraform-tests job failed destroying a real `aws_medialive_input`:
`tofu destroy` reported `Error: waiting for delete AWS Elemental MediaLive
Input (<id>): couldn't find resource (21 retries)`. `DeleteInput` removed
the record outright, so `DescribeInput` 404'd immediately.

Verified against terraform-provider-aws's own source
(`internal/service/medialive/input.go`) rather than assumed: its
`waitInputDeleted` polls `DescribeInput` via `statusInput` with
`Pending: [InputStateDeleting]`, `Target: [InputStateDeleted]`. `statusInput`
maps a `NotFoundException` to `(nil, "", nil)` -- an *indeterminate* result,
not a success -- because `retry.StateChangeConf`'s empty-target convention
(“not found means done”) does not apply here: the target is the literal
string `"DELETED"`, never empty. An immediate 404 is therefore
indistinguishable, from the waiter's perspective, from "still deleting", so
it burns its `NotFoundChecks` budget (21 polls observed) and fails instead
of succeeding. `resourceInputDelete` does treat a `NotFoundException`
returned directly from the `DeleteInput` call itself as done (idempotent
delete-of-already-gone), but that's a different code path from the
post-delete wait.

Real AWS models exactly this soft-delete window (`InputState` enum has
`CREATING`/`DETACHED`/`ATTACHED`/`DELETING`/`DELETED` -- `types/enums.go`,
aws-sdk-go-v2/service/medialive@v1.101.4): a deleted input stays describable
with `State: DELETED` for some period rather than disappearing on the same
call. Fixed by having `DeleteInput` mark the stored input `State =
stateDeleted` in place instead of removing it from the table, mirroring the
existing `stateDeleted` convention already used for Channel/Multiplex
responses (`channels.go`, `multiplexes.go`) but, unlike those, actually
leaving the record live for a subsequent `DescribeInput`. `ListInputs` and
`InputCount` are unchanged and will continue to include a DELETED input;
no test or real-client evidence required excluding it, and doing so
speculatively would be an unverified guess. `BatchDeleteInput`
(`batch.go`) still hard-deletes and was left alone -- out of scope, no
failing test or provider evidence implicates it.

Reproduced against the real HashiCorp AWS provider via `go test
./test/terraform/ -run TestTerraform_MediaLive` (`tofu destroy` failing with
the exact CI error) and against the real `aws-sdk-go-v2` client
(`TestDeleteInput_RealClient_StaysDescribableAsDeleted`,
`handler_inputs_test.go`) before the fix, both now passing after it.
`TestInput_CRUD`'s delete assertions were updated to match (input count
stays 1, Describe returns 200 with state DELETED, not 404).

## 2026-08-23: every List operation ignored the client's maxResults/nextToken

Prompted by the still-open "deep state/error-code audit ... was not
re-performed" gap for Reservation/Offering (see the `gaps` list above),
this pass audited Offering/Reservation's request side, not just their
already-`ok` wire shape. `handleListOfferings`/`handleListReservations`
called `h.Backend.ListOfferings(0, "")`/`ListReservations(0, "")`
unconditionally -- the request's actual `maxResults`/`nextToken` query
params (both real, httpQuery-bound `ListOfferingsInput`/
`ListReservationsInput` fields, confirmed against
`awsRestjson1_serializeOpHttpBindingsListReservationsInput` in
aws-sdk-go-v2/service/medialive@v1.101.4's serializers.go) were parsed by
nobody and silently discarded.

Since `pkgs/page.New`'s cursor is derived entirely from the `nextToken`
argument passed in, an always-empty `nextToken` means every call restarts
at item 0: a real client's paginator (`ListReservationsPaginator`, which by
default has no protection against a server repeating the same token --
`StopOnDuplicateToken` is off unless the caller opts in) resends whatever
`nextToken` the server just gave it, gets back the identical first page and
the identical `nextToken` again, and never terminates once a table exceeds
`defaultMaxResults` (20).

Checked the sibling: this is not a Reservation/Offering-only bug. Every
`List*` handler in the service shared the exact same
`h.Backend.ListX(0, "")` shape. Grepped for it and found 20 occurrences
across every family -- Channel, Input, InputSecurityGroup, InputDevice
(both `ListInputDevices` and `ListInputDeviceTransfers`), Multiplex,
MultiplexProgram, Cluster (both `ListClusters` and `ListClusterAlerts`),
Node, ChannelPlacementGroup, SignalMap, CloudWatchAlarmTemplate(Group),
EventBridgeRuleTemplate(Group), Offering, Reservation, Network, SdiSource
-- and confirmed each corresponding real op's serializer binds both
`maxResults` and `nextToken` as httpQuery the same way. Fixed all 20 with
the same change: a new `paginationParams(c)` helper (`handler.go`) reads
`c.QueryParam("maxResults")`/`c.QueryParam("nextToken")`, and every one of
the 20 handlers now passes those through instead of the hardcoded `(0,
"")`. Purely additive to `handler.go`'s query-parsing surface; no
persisted struct changed, so no `medialiveSnapshotVersion` bump.

Proof: `TestListReservations_RealClientPaginator_AdvancesThroughAllPages`
(`handler_reservations_pagination_test.go`) purchases 25 reservations (>
`defaultMaxResults`), drives a real `medialivesdk.NewListReservationsPaginator`
through two pages, and asserts page 2's reservation IDs are disjoint from
page 1's. Confirmed failing against the unfixed handler (hand-reverted
`handleListReservations` to its old `ListReservations(0, "")` body): every
ID on page 2 was reported as "appeared on both page 1 and page 2" for all
20 items, i.e. page 2 was a byte-for-byte repeat of page 1. Restored and
re-verified `handler_reservations.go` was byte-identical via md5sum before
and after. `go test -race ./services/medialive/...` passes with the fix in
place.

Not fixed, and called out separately rather than folded into the same fix:
`ListAlerts`/`ListMultiplexAlerts` also have real `maxResults`/`nextToken`
query params (confirmed the same way against
`awsRestjson1_serializeOpHttpBindingsListAlertsInput`/
`ListMultiplexAlertsInput`), but gopherstack's `ListAlerts`/
`ListMultiplexAlerts` backend methods (`channels.go`/`multiplexes.go`)
don't accept pagination arguments at all -- they always return every alert
unbounded, with no `nextToken` ever emitted. This doesn't reproduce the
same "stuck forever" failure mode (there's no dangling token for a real
paginator to get stuck resending), so it's a lower-severity, structural
modelling gap -- adding real truncation would mean widening both backend
method signatures and their `ListAlertsOutput`/`ListMultiplexAlertsOutput`
wire shape, out of scope for this pass. `ListVersions` and
`ListTagsForResource` were checked and confirmed to genuinely have no
`maxResults`/`nextToken` in the real API (`ListVersionsInput`/
`ListTagsForResourceInput`'s httpBindings serializers take no query params
beyond, for the latter, the required `resourceArn` URI segment) -- nothing
to fix there.

Also examined but left alone, unprovable: `DeleteReservation` hard-deletes
the reservation from `b.reservations` after briefly setting `State =
"CANCELED"` on the in-memory copy returned to the caller --
`TestReservations_PurchaseListDescribeDeleteUpdate` already asserts
`DescribeReservation` 404s immediately afterward, as a deliberate, tested
choice. The real `ReservationState` enum has a fourth value, `DELETED`,
distinct from `CANCELED`, that gopherstack's Reservation model can never
reach (`types/enums.go`,
aws-sdk-go-v2/service/medialive@v1.101.4) -- suggestive of the same
soft-delete pattern this file's Input entry above documents, but there is
no terraform-provider-aws resource for a MediaLive reservation and no CI
failure to corroborate it the way the Input fix had, so this is flagged
here as a follow-up question rather than changed.


## 2026-08-29 enum-VALUE sweep (wrapper-key-sweep campaign, wire-shape enforcement all services)

Targeted pattern hunt for the comprehend class of bug: a status/state value assigned to a
domain struct field that is not a member of the real AWS enum for the corresponding response
member, reaching the wire through the field rather than a same-site literal `cmd/enumcheck` can
resolve. Checked every domain struct field holding a status/state concept (`store.go`'s shared
`stateIdle`/`stateRunning`/`stateStopping`/`stateStarting`/`stateDeleted`/`stateDeleting`/
`stateDetached` vocabulary spans `Channel.State`/`Multiplex.State`/`Input.State`, plus dedicated
per-family constants for `Cluster`/`Node`/`Network`/`SdiSource`/`ChannelPlacementGroup`) against
the real SDK enum (`medialive@v1.101.4 types/enums.go`). `cmd/enumcheck` was run both before and
after and flagged **none** of the findings below.

**Found and fixed**: `signal_maps.go`'s `SignalMap.Status`/`MonitorDeploymentStatus` — a single
sloppy pair of literals wrong in four places, the comprehend shape (one invented vocabulary
reused across a family of ops, not matching the real per-op enum):

- `CreateSignalMap` and `StartUpdateSignalMap` both set `Status = "SUCCEEDED"`. The real member
  is `types.SignalMapStatus` (CREATE_IN_PROGRESS/CREATE_COMPLETE/CREATE_FAILED/
  UPDATE_IN_PROGRESS/UPDATE_COMPLETE/UPDATE_REVERTED/UPDATE_FAILED/READY/NOT_READY,
  `types/enums.go`), which has no `SUCCEEDED` member at all. Fixed to `"CREATE_COMPLETE"` /
  `"UPDATE_COMPLETE"` respectively (this backend has no async signal-map pipeline, so the
  immediate-terminal-state convention already used elsewhere in this file applies).
- `StartMonitorDeployment` set `MonitorDeploymentStatus = "DEPLOYED"`; `StartDeleteMonitorDeployment`
  set it to `"DELETING"`. The real member is `types.SignalMapMonitorDeploymentStatus`
  (NOT_DEPLOYED/DRY_RUN_DEPLOYMENT_*/DEPLOYMENT_COMPLETE/DEPLOYMENT_FAILED/
  DEPLOYMENT_IN_PROGRESS/DELETE_COMPLETE/DELETE_FAILED/DELETE_IN_PROGRESS) — neither `"DEPLOYED"`
  nor bare `"DELETING"` is a member. Fixed to `"DEPLOYMENT_COMPLETE"` / `"DELETE_COMPLETE"`.

Three pre-existing unit tests in `handler_signal_maps_test.go` asserted the old, wrong literals
as correct (`TestSignalMap_CRUD`'s "create returns 201 with id and SUCCEEDED status" case,
`TestSignalMap_GetListDelete`'s `"DEPLOYED"` assertion, `TestStartDeleteMonitorDeployment`'s
`"DELETING"` assertion) — all three updated to assert the real enum values instead, per this
campaign's "do not trust existing tests" rule.

**Response-nesting sweep (separate pass, same bug class as above but wire-shape depth, not a
value) — N of N ops checked for this class: all 5 ops sharing `toSignalMapOutput`
(`CreateSignalMap`/`GetSignalMap`/`StartUpdateSignalMap`/`StartMonitorDeployment`/
`StartDeleteMonitorDeployment`)**: `toSignalMapOutput` (`handler_signal_maps.go`) emitted a flat
top-level `"monitorDeploymentStatus"` key, but the real `CreateSignalMapOutput`/`GetSignalMapOutput`/
`StartUpdateSignalMapOutput`/`StartMonitorDeploymentOutput`/`StartDeleteMonitorDeploymentOutput`
all nest it as `MonitorDeployment *types.MonitorDeployment` → `.Status`
(`types/types.go:5679`, wire key `"monitorDeployment"` per
`deserializers.go:4687-4690`). A real SDK client silently discarded the flat key and decoded
`MonitorDeployment` as `nil` — losing exactly that one field (`Status`/`Arn`/`Id`/etc. all decoded
correctly; this is a one-field loss, not the total-nil-decode shape glue's sibling bug has). Fixed
by nesting: `"monitorDeployment": map[string]any{"status": sm.MonitorDeploymentStatus}`. The
sibling `toSignalMapSummary` (`ListSignalMaps`) was re-verified against `types.SignalMapSummary`
and correctly keeps `MonitorDeploymentStatus` flat — that type genuinely has no nested member, so
it was left unchanged. Verified via `TestSignalMap_MonitorDeploymentStatusIsLegalEnumMember` and
`TestCreateSignalMap_MonitorDeploymentNested` (real typed client, asserts
`.MonitorDeployment.Status` is non-nil/populated post-fix, confirmed failing pre-fix) in
`wire_field_fixes_test.go`. Four pre-existing tests asserted the flat key as correct
(`TestSignalMap_MonitorDeploymentStatusIsLegalEnumMember` — rewritten to drive the real client
rather than raw HTTP — plus `TestSignalMap_CRUD`, `TestSignalMap_GetListDelete`, and
`TestStartDeleteMonitorDeployment` in `handler_signal_maps_test.go`) — all updated to assert the
real nested shape instead.

**Checked clean** (N-of-N legal-value coverage against the real enum, no fix needed):
`ChannelState` (5/11: IDLE/STARTING/RUNNING/STOPPING/DELETED used), `MultiplexState`,
`InputState`, `ClusterState`, `NetworkState`, `SdiSourceState`, `ChannelPlacementGroupState`,
`NodeConnectionState`, `InputDeviceConnectionState`, `DeviceSettingsSyncState`,
`DeviceUpdateStatus`, `ReservationState`, `ClusterAlertState`. `nodeStateDeleted = "DELETED"`
(`store.go:54`) is DORMANT — declared but never assigned anywhere (`DeleteNode` removes the Node
from its map entirely rather than transitioning state, `UpdateNodeState`'s `state` param is
pure client-input passthrough for the real typed `types.NodeState` field) — not fixed, no
reachable path exists to manufacture without fabricating one.

Gates: `go build ./services/medialive/...` (clean), `go vet ./...` (repo-wide, clean — no
signature changes this pass), `go test -race -count=1 ./services/medialive/...` (pass, including
new `wire_field_fixes_test.go` and the three corrected pre-existing tests, each new/changed
assertion hand-verified to fail against the pre-fix literals then restored),
`golangci-lint run --fix ./services/medialive/...` (0 issues). Work left uncommitted per this
pass's instructions.

## Error-discard sweep (2026-08-29): verified clean, no bugs found

Audited every discarded-error/discarded-return-value assignment
(`x, _ := ...`, bare `_ = ...`) in non-test `.go` files -- ~149 sites --
looking for the sesv2 `SendBulkEmail` class of bug: a call whose failure had
a designated place to be reported and wasn't.

`BatchStart`, `BatchStop`, `BatchDelete` (batch.go, handler_batch.go) and
`BatchUpdateSchedule` (schedules.go, handler_schedules.go) are the only
per-item-status/batch-shaped operations in this service; all four check
their backend call's `err` return via `respondErr` and thread
`Successful`/`Failed` (or `Creates`/`Deletes`) fully into the response --
none silently drop a per-item failure.

The rest of the non-type-assertion discards are the `extractX(body) (T,
bool)` optional-field helpers feeding `extractChannelCreateExtras`/
`extractChannelUpdateExtras` (handler_channels.go, handler_clusters.go,
handler_reservations.go, handler_channels_encoder.go) -- discarding the
presence bool is correct: an absent field should leave the extra at its
zero value, which is what happens. `classifyPath`'s unused return values
(handler.go:423) are routing outputs already consumed elsewhere in the same
call.

No test changes; no source changes. Recorded as genuinely clean for this bug
class.

## 2026-08-30 gopherstack-wlo1: error-envelope re-verification (N-of-N)

Re-visited as part of a 5-service error-envelope sweep (lightsail,
medialive, pinpoint, quicksight, apigateway). The 2026-08-22 fix above was
verified via 2 sampled `deserializeOpError` functions
(`DescribeChannel`/`CreateChannel`); this pass read all 123 in
`deserializers.go` (123-of-123, not sampled) and confirms every one is
identical generated boilerplate reading `X-Amzn-ErrorType` then
`restjson.GetErrorInfo` -- the existing fix covers the whole surface, not
just the two sampled ops.

Strengthened `handler_error_type_test.go`'s existing
`TestDescribeChannel_UnknownChannelSurfacesNotFoundException` (which
asserted only the `smithy.APIError` interface + `ErrorCode()` string) with
an additional `errors.As` assertion against the concrete
`*types.NotFoundException`, and added
`TestDescribeChannel_UnknownChannelRawEnvelope` asserting on the raw
response header/body bytes directly. Both pass unmodified -- no bug found,
this service remains correctly fixed.

Gates (this pass, `services/medialive/` only): `go build`, `go vet`,
`go test -race -count=1`, `golangci-lint run` -- all clean.

## 2026-08-30 value-semantics sweep (gopherstack-uox6) -- clean, no code change

Re-audited every List/Describe operation's optional request parameters against the pinned
`medialive@v1.101.4` doc comments for the class described in gopherstack-uox6 (a parameter that
IS read and applied but with the wrong algorithm -- negation/case/operator/combining-rule/
boundary/default-meaning errors invisible to a field-shape or enum scanner). 41 List/Describe ops
counted directly from `api_op_List*.go`/`api_op_Describe*.go` filenames (24 List + 17 Describe),
matching the brief's count.

Nearly this entire surface was already closed by the prior "Constraining-parameter sweep
(wrapper-key campaign, 2026-08-29)" entry above (six real bugs fixed: ListClusterAlerts'
StateFilter, ListReservations' six filters, GroupIdentifier/SignalMapIdentifier on the CW/EB
template-group and template List ops, ListSignalMaps' two group filters,
ListInputDeviceTransfers' TransferType direction bug) -- that pass used the identical discipline
(read the SDK doc comment, check the algorithm, not just whether the field is read) even though it
predates this bd issue. This pass independently re-verified rather than trusted that entry:

- `ListAlerts`/`ListMultiplexAlerts`: confirmed `StateFilter` (SET/CLEARED/ALL) is still never read
  by `channels.go`/`multiplexes.go` -- but both backends always return `[]map[string]any{}`
  unconditionally (no `ChannelAlert`/synthetic-alert generation exists for either resource, unlike
  `ListClusterAlerts`' synthetic "cluster-not-ready" alert). No legal `StateFilter` value can ever
  change either operation's output, so this is structurally inert, not a live bug -- the same
  restraint class as `RecipeProvider` in personalize below. Already documented at the `Alerts:`
  entry above; not re-opened.
- `ReservationFilter.matches` (reservations.go): re-read against `ListReservations`'/
  `ListOfferings`' doc comments (`api_op_ListReservations.go`/`api_op_ListOfferings.go`) -- every
  filter is a plain equality string with no wildcard/negation/case-insensitivity documented; AND
  across the seven independent dimensions, correct as written.
- `findCWAlarmTemplateGroup`/`findEBRuleTemplateGroup` (id-or-ARN-or-name lookup): same helper used
  by both Create's uniqueness check and List's `GroupIdentifier`/`SignalMapIdentifier` filters --
  internally consistent, and MediaLive's own doc ("Can be either be its id or current name") is a
  subset of what's accepted (ARN also matches), not a narrower set silently excluded.
- MaxResults: every List op's doc comment is either "Placeholder documentation for MaxResults" (SDK
  codegen placeholder, not a real spec) or a bare "The maximum number of items to return" -- no
  operation in this service documents a specific default page size to check against.

No new bug found; no source or test changes this pass. Restraint already on record (ListOfferings'
10 filters against a fixed 3-item catalog, Scope's undocumented wire vocabulary) re-confirmed, not
re-litigated.

Gates: `go build ./services/medialive/...`, `go vet ./services/medialive/...` (no changes, nothing
to verify beyond confirming the tree is unchanged). Work left uncommitted per this pass's
instructions.

## 2026-08-31 wrapper-key/per-item sweep of PARITY-unnamed ops (gopherstack-6flj / -21my)

Targeted the standing shortcut: every `List*`/`Describe*` operation in `medialive@v1.101.4` whose
name never appeared anywhere in this file before today. 17 such ops (derived directly from
`api_op_List*.go`/`api_op_Describe*.go` filenames against a grep of this file, not assumed):
`DescribeAccountConfiguration`, `DescribeChannelPlacementGroup`, `DescribeCluster`,
`DescribeInputSecurityGroup`, `DescribeMultiplex`, `DescribeMultiplexProgram`, `DescribeNetwork`,
`DescribeNode`, `DescribeOffering`, `DescribeSdiSource`, `DescribeThumbnails`,
`ListChannelPlacementGroups`, `ListInputSecurityGroups`, `ListMultiplexPrograms`,
`ListMultiplexes`, `ListNetworks`, `ListSdiSources`. Protocol confirmed from
`deserializers.go` itself: `awsRestjson1` (JSON) -- no case folding, so a naming mismatch here is
always a hard failure, never a latent case-only bug.

Read each op's own deserializer function in the pinned SDK (not a sibling's, not a doc). All 6
List wrapper keys were already correct (`channelPlacementGroups`, `inputSecurityGroups`,
`multiplexPrograms`, `multiplexes`, `networks`, `sdiSources`) -- this service's earlier wrapper-key
pass covered the ops that existed at the time, and no new List op has been added since. Two
per-item bugs found, both the "list omits a field its singular sibling carries" shape:

1. `ListInputSecurityGroups`' item map never included `tags`, though
   `types.InputSecurityGroup` (the exact same wire type reused for both List and Describe, not a
   separate summary shape) declares `tags` as a real member, the backend already tracks it per
   group, and `DescribeInputSecurityGroup`/`CreateInputSecurityGroup` already emit it correctly.
   Fixed: added `Tags` to `InputSecurityGroupSummary` (`interfaces.go`), populated in
   `storedInputSecurityGroup.toSummary` (`models.go`), emitted in
   `handleListInputSecurityGroups` (`handler_input_security_groups.go`).
2. `ListMultiplexes`' item map never included `multiplexSettings` or `tags`, though
   `types.MultiplexSummary` declares both (`multiplexSettings` as a
   `*types.MultiplexSettingsSummary{TransportStreamBitrate}`), the backend tracks both per
   multiplex, and `DescribeMultiplex` already emits them correctly. Fixed: added
   `TransportStreamBitrate`/`Tags` to `MultiplexSummary` (`interfaces.go`), populated in
   `storedMultiplex.toSummary` (`models.go`), emitted in `handleListMultiplexes`
   (`handler_multiplexes.go`).

One more bug found off this axis, not list-vs-singular but singular-vs-tracked-state:
`DescribeOffering`/`PurchaseOffering`/`DescribeReservation` all build their
`resourceSpecification` object by hand and every one of them omitted `specialFeature`, even
though `OfferingResourceSpecification`/`Reservation.ResourceSpecification` already track
`SpecialFeature` (it is even read as a `ListReservations` query filter, per the
2026-08-29 entry above), and `types.ReservationResourceSpecification` declares it as a real
member. No seed offering had ever set a non-empty `SpecialFeature`, so this decoded empty for
every offering/reservation regardless of catalog data. Fixed: added `SpecialFeature:
"AUDIO_NORMALIZATION"` to the seeded `87654321` HD offering (`store.go`, flows into any
reservation purchased from it) and added the `specialFeature` key to both `toOfferingOutput` and
`toReservationOutput` (`handler_reservations.go`).

CLEAN (checked field-for-field against the pinned deserializer, not skimmed) among the 17 targeted:
`DescribeAccountConfiguration`
(wrapper `accountConfiguration`+`kmsKeyId`), `DescribeChannelPlacementGroup`/
`ListChannelPlacementGroups` (all 7 real `DescribeChannelPlacementGroupSummary` fields present),
`DescribeCluster` (all 8 real `DescribeClusterOutput` fields present via shared `toClusterOutput`),
`DescribeNetwork`/`ListNetworks` (all 7 real `DescribeNetworkSummary` fields present, plus a
harmless extra `tags` key the real summary type doesn't declare -- ignored by any real client's
decoder), `DescribeSdiSource`/`ListSdiSources` (all 7 real `SdiSourceSummary` fields present),
`ListMultiplexPrograms` (both real `MultiplexProgramSummary` fields present -- initially
mis-suspected of a bug from a spillover grep match on the *next* deserializer function in the
file; re-verified with an exact function-boundary read).

GAPS recorded, not fixed -- real per-item mismatches that no legal input can currently populate,
because nothing in this backend tracks the value:
- `InputSecurityGroup.channels`/`.inputs` (both List and Describe -- shared gap, no
  channel/input-to-security-group association tracked on the security group side).
- `DescribeMultiplex`'s top-level `destinations` (real member of `types.Multiplex`; this backend
  never models multiplex output destinations).
- `DescribeMultiplexProgram`'s `packetIdentifiersMap`, `pipelineDetails`, and
  `multiplexProgramSettings.videoSettings` (all real members with zero backing state -- no PID
  mapping or video-mux engine modeled).
- `DescribeNode`'s `instanceArn`, `nodeInterfaceMappings`, `sdiSourceMappings` (real members; no
  hardware-level node/interface modeling in this backend).
- `DescribeThumbnails` always returns an empty `thumbnailDetails` list (correct wrapper key,
  correct empty-when-unmodeled behavior -- no actual video pipeline generates thumbnail images
  here).
- `resourceSpecification.channelClass` on `Offering`/`Reservation` (real member of
  `types.ReservationResourceSpecification`; `OfferingResourceSpecification` has never modeled
  `ChannelClass`, consistent with the 2026-08-29 entry's note on the same axis).

Tests: 4 new, all in `wire_field_fixes_sweep2_test.go`, each driving the real
`aws-sdk-go-v2/service/medialive` client end-to-end and asserting on the decoded typed response
(`TestDescribeOffering_ResourceSpecification_SpecialFeature_RealClient`,
`TestDescribeReservation_ResourceSpecification_SpecialFeature_RealClient`,
`TestListInputSecurityGroups_Tags_RealClient`,
`TestListMultiplexes_SettingsAndTags_RealClient`). Seeded distinguishable non-zero values
(`"AUDIO_NORMALIZATION"`, two-key tag maps, a specific `TransportStreamBitrate`) and at least two
items where relevant. All 4 confirmed failing against unmodified code before the fix (zero
value/nil/missing key in every case), then confirmed passing after.

No case-only mismatches (impossible on this protocol -- restjson1 does not fold case). No hard
decode errors or panics found this pass. No elements emitted that are not real type members. No
wrapping-shape mismatches (every list here was already correctly member-wrapped, not flattened).
No stale `nolint` found in any edited file (`interfaces.go`'s pre-existing `dupl` suppression at
line ~1692 and `handler_multiplexes.go`'s two pre-existing `gosec` suppressions are unrelated to
this pass's edits and remain in active use per `golangci-lint run` returning 0 issues).

Gates: `go build ./services/medialive/...`, `go vet ./services/medialive/...`,
`go test ./services/medialive/... -race -count=1`, `golangci-lint run ./services/medialive/...` --
all clean. Work left uncommitted per this session's hard constraints.

## 2026-09-07 (gopherstack-b668)

`PurchaseOffering` (`reservations.go`) fabricated a frozen `Start:
"2024-01-01T00:00:00Z"`/`End: "2025-01-01T00:00:00Z"` on every purchase,
regardless of the offering's `Duration`/`DurationUnits` or the caller's
`PurchaseOfferingInput.Start` (`api_op_PurchaseOffering.go`: "Requested
reservation start time ... If no value is given, the default is now").
`OfferingDurationUnits` (`types/enums.go`) declares exactly one value,
`MONTHS`.

Fixed: `PurchaseOffering` now takes the caller's `start` (wired through from
the `"start"` request-body key, previously dropped entirely by
`handlePurchaseOffering`), defaults it to `b.now()` when omitted, and derives
`End` as `Start + Duration` months via a new `addOfferingTerm` helper. Added
`nowFunc func() time.Time` to `InMemoryBackend` (`store.go`), following the
existing `azurequeue`/`azuretable`/`cosmosdb`/`resourcegroupstaggingapi`
convention (`nowFunc` defaulting to `time.Now`, a `now()` method returning
`.UTC()`) rather than calling `time.Now()` inline, so the time source stays
overridable for tests. An invalid (non-RFC3339) explicit `start` now returns
`BadRequestException` (`ErrInvalidParameter`), a declared
`PurchaseOffering` error.

Two pre-existing tests in `handler_reservations_test.go` were asserting the
fabrication as if it were correct behavior and are corrected:
- `TestReservations_PurchaseListDescribeDeleteUpdate` asserted
  `resv["state"] == "EXPIRED"` immediately after purchase (true only because
  the fabricated `End` was always in the past by the time any real clock ran
  this test). Now asserts `"ACTIVE"` ("a term starting now hasn't ended
  yet"), and the subsequent delete call now forces the term into the past
  via `ForceReservationEnd` first (previously it worked by accident of the
  same fabrication).
- `TestReservations_DeleteRequiresExpired`'s `past_term_end_is_deletable`
  subtest relied on the same fabricated past `End` to reach `EXPIRED`
  without forcing it; now calls `ForceReservationEnd` explicitly, same as
  its sibling `still_within_term_is_rejected` subtest already did.

Tests: 2 new in `handler_reservations_test.go` --
`TestPurchaseOffering_DerivesTermFromDuration` (no `start` given: asserts
`Start` is within a minute of now and `End == Start.AddDate(0, duration,
0)`) and `TestPurchaseOffering_HonorsExplicitStart` (an explicit `start`
round-trips verbatim and `End` is exactly 12 months later). Both confirmed
failing against the unmodified fabricated-dates code (`Start`/`End` pinned
to 2024-01-01/2025-01-01, `ACTIVE` still read back as `EXPIRED`), then
passing after the fix.

Gates: `go test -race -count=1 ./services/medialive/...`,
`golangci-lint run services/medialive/...` -- both clean.

## 2026-09-07 (gopherstack-f6dz)

The b668 fix above honored a caller-supplied `PurchaseOfferingInput.Start`
but never bounded it. `api_op_PurchaseOffering.go`'s doc comment on `Start`
continues past the "default is now" sentence already quoted above: "The
specified time must be between the first day of the current month and one
year from now." A well-formed but out-of-window `Start` was accepted, so a
caller could pin a reservation term start years in the future or in the
past.

Fixed: `PurchaseOffering` (`reservations.go`) now rejects an explicit
`start` outside `[firstOfMonthUTC(b.now()), b.now().AddDate(1, 0, 0)]`
(new `firstOfMonthUTC` helper) with `BadRequestException`
(`ErrInvalidParameter`, already declared for this op). Both bounds read as
inclusive and are computed from `b.now()`, the same `nowFunc` seam b668
added -- not wall-clock, not a fixed date. A `start` of `""` (the "default
is now" path) is unaffected: `b.now()` trivially satisfies its own window.

Added `SetNow` to `export_test.go` (overrides `nowFunc`, following
`ForceReservationEnd`'s existing pattern) so the boundary can be exercised
against a controlled clock instead of wall-clock.

`TestPurchaseOffering_HonorsExplicitStart` previously asserted a hardcoded
`start: "2030-03-01T00:00:00Z"` -- a well-formed date, but one this fix
newly rejects as more than a year out, and would have started failing again
on its own well before 2030 as wall-clock caught up. Corrected to derive
`start` from `time.Now()` (+7 days) instead of a fixed year, matching
b668's own rationale for killing frozen dates.

Tests: 1 new, `TestPurchaseOffering_StartWindow`
(`handler_reservations_test.go`), table-driven over all four boundary
points against a `SetNow`-pinned clock -- first instant of the current
month (accepted), the previous month's last second (rejected), exactly one
year from now (accepted), one second past that (rejected). Rejections
assert both `http.StatusBadRequest` and the `X-Amzn-Errortype:
BadRequestException` response header, not just that an error occurred.
Confirmed failing (both rejection cases) against the guard-less code, then
passing after the fix.

Gates: `go test -race -count=1 ./services/medialive/...`,
`golangci-lint run services/medialive/...` -- both clean.

## 2026-09-07 (gopherstack-ir0p)

`CreateInputInput.SdiSources` and `UpdateInputInput.SdiSources`
(`api_op_CreateInput.go`, `api_op_UpdateInput.go`) both document only "SDI
Sources for this Input." -- no element-kind detail, no tri-state note.
`types.Input.SdiSources` carries the identical doc comment and identical
`[]string` element type, so an attach must be echoed back through
`DescribeInput`. Neither handler parsed the field: `handleCreateInput` and
`handleUpdateInput` (`handler_inputs.go`) never read `body["sdiSources"]`,
`CreateInput`/`UpdateInput` (`inputs.go`) never accepted it, and
`inputOutput` never emitted it -- so an SdiSource could never be attached
to an Input through the public API.

Identifier kind: an ID, not an ARN. `types.Input.SecurityGroups`'s doc
comment ("A list of IDs for all the Input Security Groups attached to the
input") and `UpdateInputInput.InputSecurityGroups`'s ("A list of security
groups referenced by IDs") are the two sibling attachment-list fields in
the same structs, both explicitly IDs; `types.SdiSource.Id`'s own comment
("Unique in the AWS account. The ID is the resource-id portion of the
ARN.") backs the same convention. gopherstack's own `sdiSources` table
already keys by ID (`DescribeSdiSource(sdiSourceID string)`,
`storedSdiSource.ID` from `newID()`), so IDs are also what the sibling
store already speaks.

Validation: none added. `deserializeOpErrorCreateInput` declares only
`UnknownError`, `BadGatewayException`, `BadRequestException`,
`ForbiddenException`, `GatewayTimeoutException`,
`InternalServerErrorException`, `TooManyRequestsException` --
`deserializeOpErrorUpdateInput` adds `ConflictException` and
`NotFoundException` in place of `TooManyRequestsException`. Neither doc
comment states that a nonexistent SdiSource is rejected, and this
service's own `CreateSdiSource`/`DescribeSdiSource` give no cross-op
validation precedent for CreateInput/UpdateInput to inherit. Implemented
as plain passthrough: whatever ID list is given is stored verbatim, no
existence check against the `sdiSources` table.

Update semantics: `UpdateInputInput.SdiSources`'s doc comment says nothing
about absent-vs-empty -- that stays true, and this is not a documented
AWS tri-state. An absent key must still leave the existing list untouched:
`UpdateInput`'s own `name` and `roleArn` parameters, immediately above in
the same function, already treat "" (Go's zero value for an absent form
field) as no-change, and an unconditional `SdiSources` replace would
silently wipe attachments on a plain rename -- destroying state the caller
never mentioned is a bug regardless of what AWS's doc omits, and matches
how the function's other fields already behave. (An earlier version of
this fix unconditionally replaced `SdiSources` on every update, following
`UpdateInputSecurityGroup`'s `WhitelistRules` -- the wrong precedent,
because that op's rules list *is* the entire payload, not one field among
several. Caught in review before landing; see the follow-up note below.)
Presence is decided in the handler, where the raw `map[string]any` body is
still available (`_, ok := body["sdiSources"]`), and passed down as an
explicit `sdiSourcesSet bool` -- not inferred from the slice's nilness or
length in the backend, since `extractStringSlice` always returns a
non-nil slice regardless of whether the key was present.

Fix:
- `interfaces.go`: `Input.SdiSources []string` (placed last in the struct
  -- a slice's non-pointer tail is the only field-ordering choice
  `fieldalignment` can win back once every other field is a string/map, so
  it goes after the strings, not before); `CreateInput` interface
  signature gained an `sdiSources []string` parameter, `UpdateInput`
  gained `sdiSources []string, sdiSourcesSet bool`.
- `models.go`: `storedInput.SdiSources []string` (persisted field, `toInput()`
  copies it out).
- `inputs.go`: `CreateInput` stores a defensive copy of `sdiSources` on
  create; `UpdateInput` replaces `inp.SdiSources` with a defensive copy
  only when `sdiSourcesSet` is true, leaving it untouched otherwise.
- `handler_inputs.go`: `inputOutput.SdiSources`, `toInputOutput` defaults
  it to `[]string{}` like `Tags`; `handleCreateInput` extracts
  `sdiSources` via `extractStringSlice(body, "sdiSources")` unconditionally
  (create has no prior list to preserve); `handleUpdateInput` extracts the
  same way but also computes `sdiSourcesSet` from key presence and passes
  both through.
- `persistence_test.go`: updated the one direct-backend `CreateInput` call
  site to the new 5-arg signature (compile-only change, no assertions
  touched).

Tests: 3 in `handler_inputs_test.go`, all driven through the HTTP handler
(`doRequest`), not the backend directly.
- `TestInput_SdiSources_CreateAndUpdate`: create without `sdiSources`
  yields an empty list (no documented default to assert otherwise); create
  with two distinct IDs (`sdi-aaa`, `sdi-bbb`) round-trips the exact list
  through both the create response and a subsequent `DescribeInput`;
  update *with* an explicit, wholly disjoint pair (`sdi-ccc`, `sdi-ddd`)
  replaces it exactly, with no survivor from the original list, in both
  the update response and a follow-up `DescribeInput`.
- `TestInput_SdiSources_UpdateWithoutFieldPreservesList`: create with
  (`sdi-aaa`, `sdi-bbb`), update with a body containing only `"name"` (no
  `sdiSources` key at all), asserts both entries survive in the update
  response and a follow-up `DescribeInput`. Pins the review-caught bug.
- `TestInput_SdiSources_UpdateWithExplicitEmptyClearsList`: same setup,
  update with `"sdiSources": []` (key present, empty), asserts the list is
  actually cleared -- distinguishing "absent" from "explicit empty" rather
  than conflating them.

All three confirmed failing against unmodified (pre-`gopherstack-ir0p`)
code: `sdiSources` assertions returned `actual: <nil>` throughout, proven
by reverting `interfaces.go`, `models.go`, `inputs.go`,
`handler_inputs.go`, and the `persistence_test.go` call-site edit, running
the tests, observing the failures, then restoring all five files.
`TestInput_SdiSources_UpdateWithoutFieldPreservesList` was separately
confirmed failing (`actual: []` instead of `["sdi-aaa","sdi-bbb"]`)
against the intermediate unconditional-replace version of this fix, by
neutering just the `if sdiSourcesSet` guard in `UpdateInput` back to an
unconditional replace, re-running, then restoring the guard -- the other
two tests still passed at that point, confirming they don't depend on the
guard and so weren't accidentally masking the bug.

Snapshot golden: `storedInput` is persisted and gained a field, so
`TestSnapshotVersionGuard` (`pkgs/persistence`) was run read-only (no
`-update`) and confirms exactly that -- "medialive: backendSnapshot fields
changed without a version bump; golden is out of date... this is
bookkeeping, not a version-bump case: every old field is still present
unchanged, so the diff is additive only and needs no bump." No version
bump was made; the golden needs a `-update` refresh, left to the committer
per instructions.

Gates: `go test -race -count=1 ./services/medialive/...`,
`golangci-lint run services/medialive/...` -- both clean.

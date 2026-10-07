package burp;

import javax.swing.*;
import java.awt.*;
import java.util.List;

/**
 * Per-request send dialog: project, role, agent mode, and custom instruction.
 */
final class SendOptionsDialog {
    private SendOptionsDialog() {}

    static final class Result {
        final String instruction;
        final String projectId;
        final String role;
        final KestrelClient.AgentMode agentMode;

        Result(String instruction, String projectId, String role, KestrelClient.AgentMode agentMode) {
            this.instruction = instruction;
            this.projectId = projectId == null ? "" : projectId;
            this.role = role == null ? "" : role;
            this.agentMode = agentMode;
        }
    }

    private static String lastProjectId = "";
    private static String lastRole = "";
    private static KestrelClient.AgentMode lastAgentMode = null;

    static Result show(
            Component parent,
            KestrelClient client,
            KestrelClient.Config cfg,
            String token,
            String defaultInstruction
    ) {
        KestrelClient.AgentMode defaultMode = lastAgentMode != null
                ? lastAgentMode
                : KestrelClient.AgentMode.EINO_SINGLE;

        JPanel panel = new JPanel(new BorderLayout(0, 10));
        panel.setBorder(BorderFactory.createEmptyBorder(4, 4, 0, 4));

        JPanel selectors = new JPanel(new GridBagLayout());
        GridBagConstraints gc = new GridBagConstraints();
        gc.insets = new Insets(4, 6, 4, 6);
        gc.fill = GridBagConstraints.HORIZONTAL;
        gc.weighty = 0;

        JComboBox<KestrelClient.ProjectOption> projectBox = new JComboBox<>();
        projectBox.addItem(new KestrelClient.ProjectOption("", "加载中..."));
        projectBox.setEnabled(false);

        JComboBox<KestrelClient.RoleOption> roleBox = new JComboBox<>();
        roleBox.addItem(new KestrelClient.RoleOption("", "加载中..."));
        roleBox.setEnabled(false);

        JComboBox<KestrelClient.AgentMode> agentBox = new JComboBox<>(KestrelClient.AgentMode.values());
        agentBox.setSelectedItem(defaultMode);
        agentBox.setRenderer(new DefaultListCellRenderer() {
            @Override
            public Component getListCellRendererComponent(JList<?> list, Object value, int index, boolean isSelected, boolean cellHasFocus) {
                super.getListCellRendererComponent(list, value, index, isSelected, cellHasFocus);
                if (value instanceof KestrelClient.AgentMode) {
                    setText(((KestrelClient.AgentMode) value).displayName);
                }
                return this;
            }
        });

        // Row 0: labels — 项目 | 角色 | 对话
        gc.gridy = 0;
        gc.weightx = 1.0;

        gc.gridx = 0;
        selectors.add(new JLabel("项目"), gc);
        gc.gridx = 1;
        selectors.add(new JLabel("角色"), gc);
        gc.gridx = 2;
        selectors.add(new JLabel("对话"), gc);

        // Row 1: dropdowns
        gc.gridy = 1;

        gc.gridx = 0;
        selectors.add(projectBox, gc);
        gc.gridx = 1;
        selectors.add(roleBox, gc);
        gc.gridx = 2;
        selectors.add(agentBox, gc);

        JTextArea editor = new JTextArea(
                defaultInstruction == null || defaultInstruction.trim().isEmpty()
                        ? HttpMessageFormatter.defaultInstruction()
                        : defaultInstruction,
                7,
                70
        );
        editor.setLineWrap(true);
        editor.setWrapStyleWord(true);
        editor.setFont(new Font(Font.SANS_SERIF, Font.PLAIN, 13));

        JPanel instructionPanel = new JPanel(new BorderLayout(0, 6));
        instructionPanel.add(new JLabel("测试指令（可针对当前流量修改）："), BorderLayout.NORTH);
        instructionPanel.add(new JScrollPane(editor), BorderLayout.CENTER);

        panel.add(selectors, BorderLayout.NORTH);
        panel.add(instructionPanel, BorderLayout.CENTER);

        Thread loader = new Thread(() -> {
            try {
                List<KestrelClient.ProjectOption> projects = client.fetchProjects(cfg, token);
                List<KestrelClient.RoleOption> roles = client.fetchRoles(cfg, token);
                SwingUtilities.invokeLater(() -> populateProjects(projectBox, projects, lastProjectId));
                SwingUtilities.invokeLater(() -> populateRoles(roleBox, roles, lastRole));
            } catch (Exception ex) {
                SwingUtilities.invokeLater(() -> {
                    projectBox.removeAllItems();
                    projectBox.addItem(new KestrelClient.ProjectOption("", "(加载失败)"));
                    projectBox.setEnabled(true);
                    roleBox.removeAllItems();
                    roleBox.addItem(new KestrelClient.RoleOption("", "默认"));
                    roleBox.setEnabled(true);
                    JOptionPane.showMessageDialog(parent,
                            "加载项目/角色失败: " + ex.getMessage(),
                            "Kestrel", JOptionPane.WARNING_MESSAGE);
                });
            }
        }, "Kestrel-LoadCatalog");
        loader.start();

        int result = JOptionPane.showConfirmDialog(
                parent,
                panel,
                "发送到 Kestrel",
                JOptionPane.OK_CANCEL_OPTION,
                JOptionPane.PLAIN_MESSAGE
        );
        if (result != JOptionPane.OK_OPTION) {
            return null;
        }

        KestrelClient.ProjectOption project = (KestrelClient.ProjectOption) projectBox.getSelectedItem();
        KestrelClient.RoleOption role = (KestrelClient.RoleOption) roleBox.getSelectedItem();
        KestrelClient.AgentMode agentMode = (KestrelClient.AgentMode) agentBox.getSelectedItem();
        if (agentMode == null) {
            agentMode = defaultMode;
        }

        String instruction = editor.getText();
        if (instruction == null || instruction.trim().isEmpty()) {
            instruction = HttpMessageFormatter.defaultInstruction();
        } else {
            instruction = instruction.trim();
        }

        lastProjectId = project != null ? project.id : "";
        lastRole = role != null ? role.name : "";
        lastAgentMode = agentMode;

        return new Result(instruction, lastProjectId, lastRole, agentMode);
    }

    private static void populateProjects(JComboBox<KestrelClient.ProjectOption> box,
                                         List<KestrelClient.ProjectOption> items,
                                         String selectId) {
        box.removeAllItems();
        for (KestrelClient.ProjectOption item : items) {
            box.addItem(item);
        }
        selectComboById(box, selectId);
        box.setEnabled(true);
    }

    private static void populateRoles(JComboBox<KestrelClient.RoleOption> box,
                                      List<KestrelClient.RoleOption> items,
                                      String selectName) {
        box.removeAllItems();
        for (KestrelClient.RoleOption item : items) {
            box.addItem(item);
        }
        selectComboByName(box, selectName);
        box.setEnabled(true);
    }

    private static void selectComboById(JComboBox<KestrelClient.ProjectOption> box, String id) {
        if (id == null) id = "";
        for (int i = 0; i < box.getItemCount(); i++) {
            KestrelClient.ProjectOption item = box.getItemAt(i);
            if (item != null && id.equals(item.id)) {
                box.setSelectedIndex(i);
                return;
            }
        }
        box.setSelectedIndex(0);
    }

    private static void selectComboByName(JComboBox<KestrelClient.RoleOption> box, String name) {
        if (name == null) name = "";
        for (int i = 0; i < box.getItemCount(); i++) {
            KestrelClient.RoleOption item = box.getItemAt(i);
            if (item != null && name.equals(item.name)) {
                box.setSelectedIndex(i);
                return;
            }
        }
        box.setSelectedIndex(0);
    }
}

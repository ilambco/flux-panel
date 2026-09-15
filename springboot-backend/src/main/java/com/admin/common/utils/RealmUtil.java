package com.admin.common.utils;

import com.admin.common.dto.GostDto;
import com.admin.entity.Forward;
import com.alibaba.fastjson.JSONObject;

/** Sends the small, allow-listed Realm lifecycle protocol to a node. */
public final class RealmUtil {
    private RealmUtil() {}

    public static GostDto apply(Long nodeId, Forward forward, String serviceName) {
        JSONObject data = request(forward);
        data.put("name", serviceName);
        return WebSocketServer.send_msg(nodeId, data, "ApplyRealm");
    }

    public static GostDto pause(Long nodeId, Long forwardId) {
        return WebSocketServer.send_msg(nodeId, request(forwardId), "PauseRealm");
    }

    public static GostDto resume(Long nodeId, Long forwardId) {
        return WebSocketServer.send_msg(nodeId, request(forwardId), "ResumeRealm");
    }

    public static GostDto delete(Long nodeId, Long forwardId) {
        return WebSocketServer.send_msg(nodeId, request(forwardId), "DeleteRealm");
    }

    private static JSONObject request(Forward forward) {
        JSONObject data = request(forward.getId());
        data.put("listenPort", forward.getInPort());
        data.put("remote", forward.getRemoteAddr());
        return data;
    }

    private static JSONObject request(Long forwardId) {
        JSONObject data = new JSONObject();
        data.put("id", String.valueOf(forwardId));
        return data;
    }
}
